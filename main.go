package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/PancyStudios/PancyScreenShots/pkg/api"
	"github.com/PancyStudios/PancyScreenShots/pkg/browser"
	"github.com/PancyStudios/PancyScreenShots/pkg/discord"
	"github.com/PancyStudios/PancyScreenShots/pkg/metrics"
	"github.com/PancyStudios/PancyScreenShots/pkg/queue"
	"github.com/PancyStudios/PancyScreenShots/pkg/storage"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	_ = godotenv.Load()

	// ── Storage (SQLite) ──────────────────────────────────────────────────────
	if err := storage.Init(); err != nil {
		log.Fatalf("Error inicializando base de datos: %v", err)
	}
	defer storage.Close()

	// ── Discord ───────────────────────────────────────────────────────────────
	discord.Init()

	// ── Node.js NSFW/AI Microservice ──────────────────────────────────────────
	log.Println("Iniciando microservicio de IA (Node.js)...")
	nodeCmd := exec.Command("node", "nsfw-detector/server.js")
	nodeCmd.Stdout = os.Stdout
	nodeCmd.Stderr = os.Stderr
	if err := nodeCmd.Start(); err != nil {
		log.Fatalf("Error iniciando microservicio de IA: %v", err)
	}
	defer func() {
		log.Println("Apagando microservicio de IA...")
		if nodeCmd.Process != nil {
			_ = nodeCmd.Process.Kill()
		}
	}()

	// Give Node.js 2 s to load models into memory
	time.Sleep(2 * time.Second)

	// ── Browsers ──────────────────────────────────────────────────────────────
	browser.InitBrowsers()
	defer browser.CloseBrowsers()

	// ── Worker Pool ───────────────────────────────────────────────────────────
	queue.InitWorkerPool(3)

	// ── Prometheus Metrics Server (separate port) ─────────────────────────────
	go func() {
		metricsPort := os.Getenv("METRICS_PORT")
		if metricsPort == "" {
			metricsPort = "9090"
		}
		mux := http.NewServeMux()
		mux.Handle("/metrics", metrics.Handler())
		log.Printf("Servidor de métricas escuchando en :%s/metrics", metricsPort)
		if err := http.ListenAndServe(":"+metricsPort, mux); err != nil {
			log.Printf("Error en servidor de métricas: %v", err)
		}
	}()

	// ── HTTP Server (Fiber) ───────────────────────────────────────────────────
	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
	})

	app.Use(logger.New())

	// Rate limiter: keyed by user_id (body JSON) if present, otherwise by IP.
	app.Use(limiter.New(limiter.Config{
		Max:        15,
		Expiration: 30 * time.Second,
		KeyGenerator: func(c *fiber.Ctx) string {
			if c.Method() == "POST" {
				var body struct {
					UserID string `json:"user_id"`
				}
				if err := json.Unmarshal(c.Body(), &body); err == nil && body.UserID != "" {
					return "user:" + body.UserID
				}
			}
			return "ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Demasiadas peticiones. Por favor, espera un momento.",
			})
		},
	}))

	// ── Public routes ─────────────────────────────────────────────────────────
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":     "ok",
			"version":    Version,
			"build_time": BuildTime,
		})
	})

	// ── Auth middleware ────────────────────────────────────────────────────────
	app.Use(func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		expectedToken := os.Getenv("AUTH_SCREENSHOTS")
		if expectedToken != "" && authHeader != "Bearer "+expectedToken {
			return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
		}
		return c.Next()
	})

	// ── Screenshot routes ─────────────────────────────────────────────────────
	app.Post("/api/private/screenshot/sfw", api.HandleScreenshot)
	app.Post("/api/private/screenshot/nsfw", api.HandleScreenshot)
	app.Post("/api/private/screenshot/pdf", api.HandlePDF)
	app.Post("/api/private/screenshot/gif", api.HandleGIF)
	app.Post("/api/private/screenshot/element", api.HandleElementScreenshot)
	app.Post("/api/private/screenshot/batch", api.HandleBatch)

	// ── AI / IA routes ────────────────────────────────────────────────────────
	app.Post("/api/private/ia/fetch", api.HandleIAFetch)
	app.Post("/api/private/ai/nsfw", api.HandleAINSFW)
	app.Post("/api/private/ai/toxicity", api.HandleAIToxicity)
	app.Post("/api/private/ai/scam", api.HandleAIScam)
	app.Post("/api/private/ai/scam/train", api.HandleAIScamTrain)
	app.Post("/api/private/ai/ocr", api.HandleAIOCR)
	app.Post("/api/private/ai/spam", api.HandleAISpam)

	// ── Queue & monitoring ────────────────────────────────────────────────────
	app.Get("/api/private/queue/status", api.HandleQueueStatus)

	// ── History ───────────────────────────────────────────────────────────────
	app.Get("/api/private/history/:user_id", api.HandleGetHistory)

	// ── Webhook management ────────────────────────────────────────────────────
	app.Post("/api/private/webhook/register", api.HandleRegisterWebhook)
	app.Get("/api/private/webhook/:user_id", api.HandleGetWebhook)
	app.Delete("/api/private/webhook/:user_id", api.HandleUnregisterWebhook)

	// ── Start Fiber ───────────────────────────────────────────────────────────
	go func() {
		port := os.Getenv("PORT")
		if port == "" {
			port = "3000"
		}
		if err := app.Listen(":" + port); err != nil {
			log.Fatalf("Error iniciando servidor web: %v", err)
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Apagando PancyScreenAPI...")
	discord.Close()
	_ = app.Shutdown()
}
