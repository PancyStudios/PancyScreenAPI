package main

import (
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/PancyStudios/PancyScreenShots/pkg/api"
	"github.com/PancyStudios/PancyScreenShots/pkg/browser"
	"github.com/PancyStudios/PancyScreenShots/pkg/discord"
	"github.com/PancyStudios/PancyScreenShots/pkg/queue"
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

	// Iniciar cliente de Discord
	discord.Init()
	// 0. Iniciar microservicio Node.js
	log.Println("Iniciando detector NSFW (Node.js)...")
	nodeCmd := exec.Command("node", "nsfw-detector/server.js")
	nodeCmd.Stdout = os.Stdout
	nodeCmd.Stderr = os.Stderr
	if err := nodeCmd.Start(); err != nil {
		log.Fatalf("Error iniciando el detector NSFW: %v", err)
	}

	// Asegurarse de matar el proceso Node.js al apagar Go
	defer func() {
		log.Println("Apagando detector NSFW...")
		if nodeCmd.Process != nil {
			_ = nodeCmd.Process.Kill()
		}
	}()

	// Darle 2 segundos a Node para que cargue el modelo en memoria
	time.Sleep(2 * time.Second)

	// 1. Iniciar navegadores en background
	browser.InitBrowsers()
	defer browser.CloseBrowsers()

	// 1.5 Iniciar worker pool con 3 workers
	queue.InitWorkerPool(3)

	// 2. Configurar servidor HTTP
	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
	})

	app.Use(logger.New())

	// Middleware de Límite de Velocidad (Rate Limiting)
	app.Use(limiter.New(limiter.Config{
		Max:        15,               // Máximo 15 peticiones
		Expiration: 30 * time.Second, // Por cada 30 segundos
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP() // Limitar por IP
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Demasiadas peticiones. Por favor, espera un momento.",
			})
		},
	}))

	// Endpoint de Salud (Público, sin autorización necesaria)
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":     "ok",
			"version":    Version,
			"build_time": BuildTime,
		})
	})

	// Middleware de autenticación (sencillo con Bearer token)
	app.Use(func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		expectedToken := os.Getenv("AUTH_SCREENSHOTS")

		if expectedToken != "" && authHeader != "Bearer "+expectedToken {
			return c.Status(401).JSON(fiber.Map{"error": "No autorizado"})
		}
		return c.Next()
	})

	// Rutas
	app.Post("/api/private/screenshot/sfw", api.HandleScreenshot)
	app.Post("/api/private/screenshot/nsfw", api.HandleScreenshot)
	app.Post("/api/private/ia/fetch", api.HandleIAFetch)
	// Rutas de Inteligencia Artificial (Proxies al microservicio Node.js)
	app.Post("/api/private/ai/nsfw", api.HandleAINSFW)
	app.Post("/api/private/ai/toxicity", api.HandleAIToxicity)
	app.Post("/api/private/ai/scam", api.HandleAIScam)
	// Start server en goroutine
	go func() {
		port := os.Getenv("PORT")
		if port == "" {
			port = "3000"
		}
		if err := app.Listen(":" + port); err != nil {
			log.Fatalf("Error iniciando servidor web: %v", err)
		}
	}()

	// Esperar CTRL+C para un apagado limpio
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Apagando servidor PancyScreenShots...")
	discord.Close()
	_ = app.Shutdown()
}
