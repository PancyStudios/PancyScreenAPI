package api

import (
	"github.com/PancyStudios/PancyScreenShots/pkg/queue"
	"github.com/gofiber/fiber/v2"
)

// HandleQueueStatus returns a real-time snapshot of the worker pool state.
// Route: GET /api/private/queue/status
func HandleQueueStatus(c *fiber.Ctx) error {
	return c.JSON(queue.GetStats())
}
