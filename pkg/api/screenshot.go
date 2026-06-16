package api

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/PancyStudios/PancyScreenShots/pkg/queue"
	"github.com/gofiber/fiber/v2"
)

var (
	bannedUsers = make(map[string]time.Time)
	bannedMutex sync.RWMutex

	// imageCache stores recent screenshots to avoid opening Chrome for repeated URLs
	imageCache sync.Map
)

type cacheEntry struct {
	Image     []byte
	ExpiresAt time.Time
}

// validateSecurity checks if the URL is safe to screenshot (prevents SSRF and local file access)
func validateSecurity(rawURL string) error {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return errors.New("URL malformada")
	}

	// 1. Check scheme
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("Protocolo no permitido (solo http/https)")
	}

	// 2. Check Host
	host := u.Hostname()
	if host == "localhost" {
		return errors.New("Acceso a localhost denegado")
	}

	// 3. Resolve IP and check if private
	ips, err := net.LookupIP(host)
	if err != nil {
		// Some sites might fail to resolve if blocked, but usually it's fine.
		return errors.New("no se pudo resolver el dominio")
	}

	for _, ip := range ips {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return errors.New("acceso a IPs privadas/locales denegado por seguridad")
		}
	}

	return nil
}

type ScreenshotRequest struct {
	URL    string `json:"url"`
	UserID string `json:"user_id"`
}

// HandleScreenshot takes a screenshot of the requested URL
func HandleScreenshot(c *fiber.Ctx) error {
	var req ScreenshotRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "JSON inválido"})
	}

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
					"error": fmt.Sprintf("Estás temporalmente baneado de usar este servicio por intentos de vulneración. Intenta de nuevo en %d minutos.", remaining),
				})
			} else {
				// Ban expirado
				bannedMutex.Lock()
				delete(bannedUsers, req.UserID)
				bannedMutex.Unlock()
			}
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
		return c.Status(423).JSON(fiber.Map{"error": err.Error() + " - Has sido baneado por 20 minutos."})
	}

	// Determine if this is the SFW route
	isSFW := c.Path() == "/api/private/screenshot/sfw"
	
	// Create a unique cache key based on URL and SFW requirement
	cacheKey := fmt.Sprintf("%s|%v", req.URL, isSFW)

	// Check Cache
	if entry, ok := imageCache.Load(cacheKey); ok {
		ce := entry.(cacheEntry)
		if time.Now().Before(ce.ExpiresAt) {
			// Return cached image
			c.Set("Content-Type", "image/png")
			return c.Send(ce.Image)
		} else {
			imageCache.Delete(cacheKey)
		}
	}

	resultChan := make(chan queue.JobResult)
	queue.AddJob(queue.Job{
		URL:        req.URL,
		IsSFWOnly:  isSFW,
		ResultChan: resultChan,
	})

	// Wait for the job to finish
	res := <-resultChan
	if res.Error != nil {
		if res.Error.Error() == "nsfw_content_detected" || res.Error.Error() == "nsfw_content_detected_by_dns" {
			return c.Status(403).JSON(fiber.Map{"error": "Contenido NSFW detectado. Solo permitido en la ruta NSFW."})
		}
		return c.Status(500).JSON(fiber.Map{"error": res.Error.Error()})
	}

	// Save to cache for 30 minutes
	imageCache.Store(cacheKey, cacheEntry{
		Image:     res.Image,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})

	// Devuelve la imagen como response binario
	c.Set("Content-Type", "image/png")
	return c.Send(res.Image)
}
