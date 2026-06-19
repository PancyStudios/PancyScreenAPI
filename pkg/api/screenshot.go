package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/PancyStudios/PancyScreenShots/pkg/cache"
	"github.com/PancyStudios/PancyScreenShots/pkg/discord"
	"github.com/PancyStudios/PancyScreenShots/pkg/metrics"
	"github.com/PancyStudios/PancyScreenShots/pkg/queue"
	"github.com/PancyStudios/PancyScreenShots/pkg/storage"
	"github.com/google/uuid"
	"github.com/gofiber/fiber/v2"
)

var (
	bannedUsers = make(map[string]time.Time)
	bannedMutex sync.RWMutex

	// screenshotCache is the shared cache instance for all API handlers.
	screenshotCache cache.Cache
	cacheOnce       sync.Once
)

// getCache returns the singleton Cache instance, initializing it on first call.
func getCache() cache.Cache {
	cacheOnce.Do(func() {
		screenshotCache = cache.NewCache()
	})
	return screenshotCache
}

// validateSecurity checks if the URL is safe to screenshot (prevents SSRF and local file access).
func validateSecurity(rawURL string) error {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return errors.New("URL malformada")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("Protocolo no permitido (solo http/https)")
	}

	host := u.Hostname()
	if host == "localhost" {
		return errors.New("Acceso a localhost denegado")
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return errors.New("no se pudo resolver el dominio")
	}

	for _, ip := range ips {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return errors.New("acceso a IPs privadas/locales denegado por seguridad")
		}
	}

	return nil
}

// ───────────────────────────────────────────────────────────────────────────────
// Screenshot Handler
// ───────────────────────────────────────────────────────────────────────────────

// ScreenshotRequest holds all parameters for a screenshot request.
type ScreenshotRequest struct {
	URL      string `json:"url"`
	UserID   string `json:"user_id"`
	Width    int    `json:"width"`     // optional viewport width
	Height   int    `json:"height"`    // optional viewport height
	FullPage bool   `json:"full_page"` // optional full-page capture
}

// HandleScreenshot takes a screenshot of the requested URL.
// Supports optional viewport dimensions and full-page capture.
// If the caller has a registered webhook, the job is processed asynchronously.
func HandleScreenshot(c *fiber.Ctx) error {
	start := time.Now()

	var req ScreenshotRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "JSON inválido"})
	}

	// Ban check
	if req.UserID != "" {
		bannedMutex.RLock()
		banTime, isBanned := bannedUsers[req.UserID]
		bannedMutex.RUnlock()

		if isBanned {
			if time.Now().Before(banTime) {
				remaining := int(time.Until(banTime).Minutes())
				if remaining == 0 {
					remaining = 1
				}
				return c.Status(403).JSON(fiber.Map{
					"error": fmt.Sprintf("Estás temporalmente baneado por intentos de vulneración. Intenta de nuevo en %d minutos.", remaining),
				})
			}
			bannedMutex.Lock()
			delete(bannedUsers, req.UserID)
			bannedMutex.Unlock()
		}
	}

	if req.URL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Se requiere una URL"})
	}

	if err := validateSecurity(req.URL); err != nil {
		if req.UserID != "" {
			bannedMutex.Lock()
			bannedUsers[req.UserID] = time.Now().Add(20 * time.Minute)
			bannedMutex.Unlock()
		}
		isSFW := c.Path() == "/api/private/screenshot/sfw"
		discord.SendErrorLog(req.URL, req.UserID, "Bloqueo SSRF/Local: "+err.Error(), isSFW)
		metrics.IncrErrors("screenshot", "ssrf")
		return c.Status(423).JSON(fiber.Map{"error": err.Error() + " - Has sido baneado por 20 minutos."})
	}

	isSFW := c.Path() == "/api/private/screenshot/sfw"
	screenshotType := "nsfw"
	if isSFW {
		screenshotType = "sfw"
	}

	discord.SendTakeLog(req.URL, req.UserID, isSFW)

	cacheKey := fmt.Sprintf("%s|%v", req.URL, isSFW)

	// Serve from cache if available
	if data, ok := getCache().Get(cacheKey); ok {
		metrics.IncrScreenshots(screenshotType, true)
		metrics.IncrCacheHits()
		metrics.ObserveDuration(screenshotType, time.Since(start).Seconds())
		_ = storage.AddHistory(req.UserID, req.URL, screenshotType, true)
		discord.SendSuccessLog(req.URL, req.UserID, isSFW, data)
		c.Set("Content-Type", "image/png")
		return c.Send(data)
	}

	// Webhook async mode: if the user has a registered webhook, respond 202 and deliver later
	if req.UserID != "" {
		if webhookURL, hasWebhook := storage.GetWebhook(req.UserID); hasWebhook {
			jobID := uuid.New().String()
			go func() {
				resultChan := make(chan queue.JobResult, 1)
				queue.AddJob(queue.Job{
					Type:       queue.JobTypeScreenshot,
					URL:        req.URL,
					IsSFWOnly:  isSFW,
					Width:      req.Width,
					Height:     req.Height,
					FullPage:   req.FullPage,
					ResultChan: resultChan,
				})
				res := <-resultChan
				if res.Error != nil {
					storage.DeliverWebhook(webhookURL, fiber.Map{
						"job_id":  jobID,
						"success": false,
						"error":   res.Error.Error(),
					})
					return
				}
				getCache().Set(cacheKey, res.Data, 30*time.Minute)
				_ = storage.AddHistory(req.UserID, req.URL, screenshotType, false)
				storage.DeliverWebhook(webhookURL, fiber.Map{
					"job_id":    jobID,
					"success":   true,
					"url":       req.URL,
					"type":      screenshotType,
					"image_b64": base64.StdEncoding.EncodeToString(res.Data),
				})
			}()
			return c.Status(202).JSON(fiber.Map{
				"message": "Trabajo encolado. El resultado se entregará al webhook registrado.",
				"job_id":  jobID,
			})
		}
	}

	// Synchronous mode
	resultChan := make(chan queue.JobResult, 1)
	queue.AddJob(queue.Job{
		Type:       queue.JobTypeScreenshot,
		URL:        req.URL,
		IsSFWOnly:  isSFW,
		Width:      req.Width,
		Height:     req.Height,
		FullPage:   req.FullPage,
		ResultChan: resultChan,
	})

	res := <-resultChan
	if res.Error != nil {
		if res.Error.Error() == "nsfw_content_detected" || res.Error.Error() == "nsfw_content_detected_by_dns" {
			discord.SendErrorLog(req.URL, req.UserID, "Contenido NSFW detectado", isSFW)
			metrics.IncrErrors(screenshotType, "nsfw")
			return c.Status(403).JSON(fiber.Map{"error": "Contenido NSFW detectado. Solo permitido en la ruta NSFW."})
		}
		discord.SendErrorLog(req.URL, req.UserID, "Error interno: "+res.Error.Error(), isSFW)
		metrics.IncrErrors(screenshotType, "worker")
		return c.Status(500).JSON(fiber.Map{"error": res.Error.Error()})
	}

	getCache().Set(cacheKey, res.Data, 30*time.Minute)
	_ = storage.AddHistory(req.UserID, req.URL, screenshotType, false)

	discord.SendSuccessLog(req.URL, req.UserID, isSFW, res.Data)
	metrics.IncrScreenshots(screenshotType, false)
	metrics.ObserveDuration(screenshotType, time.Since(start).Seconds())

	c.Set("Content-Type", "image/png")
	return c.Send(res.Data)
}
