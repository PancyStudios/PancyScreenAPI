package api

import (
	"archive/zip"
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/PancyStudios/PancyScreenShots/pkg/discord"
	"github.com/PancyStudios/PancyScreenShots/pkg/metrics"
	"github.com/PancyStudios/PancyScreenShots/pkg/queue"
	"github.com/gofiber/fiber/v2"
)

const maxBatchURLs = 10

// BatchRequest holds the request body for batch screenshot processing.
type BatchRequest struct {
	URLs   []string `json:"urls"`
	UserID string   `json:"user_id"`
	SFW    bool     `json:"sfw"`
}

type batchJobResult struct {
	idx int
	url string
	data []byte
	err  error
}

// HandleBatch takes screenshots of multiple URLs in parallel and returns them as a ZIP archive.
// Route: POST /api/private/screenshot/batch
func HandleBatch(c *fiber.Ctx) error {
	start := time.Now()

	var req BatchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "JSON inválido"})
	}
	if len(req.URLs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Se requiere al menos una URL en el campo 'urls'"})
	}
	if len(req.URLs) > maxBatchURLs {
		return c.Status(400).JSON(fiber.Map{
			"error": fmt.Sprintf("Máximo %d URLs por batch", maxBatchURLs),
		})
	}

	// Validate all URLs before dispatching any job
	for _, u := range req.URLs {
		if err := validateSecurity(u); err != nil {
			return c.Status(423).JSON(fiber.Map{
				"error": fmt.Sprintf("URL bloqueada por seguridad (%s): %s", u, err.Error()),
			})
		}
	}

	discord.SendTakeLog(fmt.Sprintf("BATCH: %d URLs", len(req.URLs)), req.UserID, req.SFW)

	// Dispatch all jobs (using buffered channels so AddJob doesn't block on receive)
	resultChans := make([]chan queue.JobResult, len(req.URLs))
	for i, u := range req.URLs {
		cacheKey := fmt.Sprintf("%s|%v", u, req.SFW)
		ch := make(chan queue.JobResult, 1)
		resultChans[i] = ch

		if data, ok := getCache().Get(cacheKey); ok {
			metrics.IncrCacheHits()
			ch <- queue.JobResult{Data: data}
			continue
		}

		queue.AddJob(queue.Job{
			Type:       queue.JobTypeScreenshot,
			URL:        u,
			IsSFWOnly:  req.SFW,
			ResultChan: ch,
		})
	}

	// Collect results concurrently
	results := make([]batchJobResult, len(req.URLs))
	var wg sync.WaitGroup
	for i, ch := range resultChans {
		wg.Add(1)
		go func(idx int, c chan queue.JobResult) {
			defer wg.Done()
			res := <-c
			results[idx] = batchJobResult{
				idx:  idx,
				url:  req.URLs[idx],
				data: res.Data,
				err:  res.Error,
			}
		}(i, ch)
	}
	wg.Wait()

	// Assemble ZIP archive
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for i, result := range results {
		if result.err != nil {
			metrics.IncrErrors("batch", "worker")
			f, _ := w.Create(fmt.Sprintf("error_%02d.txt", i+1))
			_, _ = f.Write([]byte(fmt.Sprintf("Error capturando %s:\n%s", result.url, result.err.Error())))
			continue
		}
		f, err := w.Create(fmt.Sprintf("screenshot_%02d.png", i+1))
		if err != nil {
			continue
		}
		_, _ = f.Write(result.data)

		// Cache individual result
		cacheKey := fmt.Sprintf("%s|%v", result.url, req.SFW)
		getCache().Set(cacheKey, result.data, 30*time.Minute)
	}
	_ = w.Close()

	metrics.IncrScreenshots("batch", false)
	metrics.ObserveDuration("batch", time.Since(start).Seconds())

	c.Set("Content-Type", "application/zip")
	c.Set("Content-Disposition", `attachment; filename="screenshots.zip"`)
	return c.Send(buf.Bytes())
}
