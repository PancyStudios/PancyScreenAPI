package api

import (
	"fmt"
	"time"

	"github.com/PancyStudios/PancyScreenShots/pkg/discord"
	"github.com/PancyStudios/PancyScreenShots/pkg/metrics"
	"github.com/PancyStudios/PancyScreenShots/pkg/queue"
	"github.com/gofiber/fiber/v2"
)

// ElementRequest holds the request body for element-scoped screenshots.
type ElementRequest struct {
	URL      string `json:"url"`
	UserID   string `json:"user_id"`
	Selector string `json:"selector"`
	SFW      bool   `json:"sfw"`
}

// HandleElementScreenshot captures a specific CSS element from a URL.
// Route: POST /api/private/screenshot/element
func HandleElementScreenshot(c *fiber.Ctx) error {
	start := time.Now()

	var req ElementRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "JSON inválido"})
	}
	if req.URL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Se requiere una URL"})
	}
	if req.Selector == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Se requiere un selector CSS (campo 'selector')"})
	}

	if err := validateSecurity(req.URL); err != nil {
		discord.SendErrorLog(req.URL, req.UserID, "SSRF bloqueado (element): "+err.Error(), req.SFW)
		metrics.IncrErrors("element", "ssrf")
		return c.Status(423).JSON(fiber.Map{"error": err.Error()})
	}

	// Check cache
	cacheKey := fmt.Sprintf("element|%s|%s|%v", req.URL, req.Selector, req.SFW)
	if data, ok := getCache().Get(cacheKey); ok {
		metrics.IncrScreenshots("element", true)
		metrics.IncrCacheHits()
		metrics.ObserveDuration("element", time.Since(start).Seconds())
		c.Set("Content-Type", "image/png")
		return c.Send(data)
	}

	resultChan := make(chan queue.JobResult, 1)
	queue.AddJob(queue.Job{
		Type:       queue.JobTypeElement,
		URL:        req.URL,
		IsSFWOnly:  req.SFW,
		Selector:   req.Selector,
		ResultChan: resultChan,
	})

	res := <-resultChan
	if res.Error != nil {
		discord.SendErrorLog(req.URL, req.UserID, "Error element: "+res.Error.Error(), req.SFW)
		metrics.IncrErrors("element", "worker")
		return c.Status(500).JSON(fiber.Map{"error": res.Error.Error()})
	}

	getCache().Set(cacheKey, res.Data, 30*time.Minute)
	metrics.IncrScreenshots("element", false)
	metrics.ObserveDuration("element", time.Since(start).Seconds())

	c.Set("Content-Type", "image/png")
	return c.Send(res.Data)
}
