package api

import (
	"github.com/PancyStudios/PancyScreenShots/pkg/storage"
	"github.com/gofiber/fiber/v2"
)

// HandleGetHistory returns the screenshot history for a specific Discord user.
// Route: GET /api/private/history/:user_id
func HandleGetHistory(c *fiber.Ctx) error {
	userID := c.Params("user_id")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Se requiere un user_id en la ruta"})
	}

	entries, err := storage.GetHistory(userID, 50)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error obteniendo historial: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"user_id": userID,
		"count":   len(entries),
		"history": entries,
	})
}
