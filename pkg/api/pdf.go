package api

import (
	"fmt"
	"time"

	"github.com/PancyStudios/PancyScreenShots/pkg/discord"
	"github.com/PancyStudios/PancyScreenShots/pkg/metrics"
	"github.com/PancyStudios/PancyScreenShots/pkg/queue"
	"github.com/gofiber/fiber/v2"
)

// PDFRequest holds the request body for PDF generation.
type PDFRequest struct {
	URL    string `json:"url"`
	UserID string `json:"user_id"`
}

// HandlePDF captures a URL and returns it as a PDF document.
// Route: POST /api/private/screenshot/pdf
func HandlePDF(c *fiber.Ctx) error {
	start := time.Now()

	var req PDFRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "JSON inválido"})
	}
	if req.URL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Se requiere una URL"})
	}

	if err := validateSecurity(req.URL); err != nil {
		discord.SendErrorLog(req.URL, req.UserID, "SSRF bloqueado (PDF): "+err.Error(), true)
		metrics.IncrErrors("pdf", "ssrf")
		return c.Status(423).JSON(fiber.Map{"error": err.Error()})
	}

	// Check cache
	cacheKey := fmt.Sprintf("pdf|%s", req.URL)
	if data, ok := getCache().Get(cacheKey); ok {
		metrics.IncrScreenshots("pdf", true)
		metrics.IncrCacheHits()
		metrics.ObserveDuration("pdf", time.Since(start).Seconds())
		c.Set("Content-Type", "application/pdf")
		c.Set("Content-Disposition", `inline; filename="page.pdf"`)
		return c.Send(data)
	}

	resultChan := make(chan queue.JobResult, 1)
	queue.AddJob(queue.Job{
		Type:       queue.JobTypePDF,
		URL:        req.URL,
		IsSFWOnly:  false,
		ResultChan: resultChan,
	})

	res := <-resultChan
	if res.Error != nil {
		discord.SendErrorLog(req.URL, req.UserID, "Error PDF: "+res.Error.Error(), false)
		metrics.IncrErrors("pdf", "worker")
		return c.Status(500).JSON(fiber.Map{"error": res.Error.Error()})
	}

	getCache().Set(cacheKey, res.Data, 30*time.Minute)
	metrics.IncrScreenshots("pdf", false)
	metrics.ObserveDuration("pdf", time.Since(start).Seconds())

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", `inline; filename="page.pdf"`)
	return c.Send(res.Data)
}
