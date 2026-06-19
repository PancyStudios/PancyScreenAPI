package api

import (
	"github.com/PancyStudios/PancyScreenShots/pkg/storage"
	"github.com/gofiber/fiber/v2"
)

// WebhookRegisterRequest is the body for registering or updating a webhook.
type WebhookRegisterRequest struct {
	UserID     string `json:"user_id"`
	WebhookURL string `json:"webhook_url"`
}

// HandleRegisterWebhook registers or updates a webhook URL for a Discord user.
// Route: POST /api/private/webhook/register
func HandleRegisterWebhook(c *fiber.Ctx) error {
	var req WebhookRegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "JSON inválido"})
	}
	if req.UserID == "" || req.WebhookURL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Se requieren los campos 'user_id' y 'webhook_url'"})
	}

	if err := storage.RegisterWebhook(req.UserID, req.WebhookURL); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error registrando webhook: " + err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{
		"message":     "Webhook registrado exitosamente",
		"user_id":     req.UserID,
		"webhook_url": req.WebhookURL,
	})
}

// HandleUnregisterWebhook removes a user's webhook registration.
// Route: DELETE /api/private/webhook/:user_id
func HandleUnregisterWebhook(c *fiber.Ctx) error {
	userID := c.Params("user_id")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Se requiere un user_id en la ruta"})
	}

	if err := storage.UnregisterWebhook(userID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error eliminando webhook: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "Webhook eliminado exitosamente",
		"user_id": userID,
	})
}

// HandleGetWebhook returns the registered webhook URL for a user.
// Route: GET /api/private/webhook/:user_id
func HandleGetWebhook(c *fiber.Ctx) error {
	userID := c.Params("user_id")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Se requiere un user_id en la ruta"})
	}

	webhookURL, ok := storage.GetWebhook(userID)
	if !ok {
		return c.Status(404).JSON(fiber.Map{"error": "No hay webhook registrado para este usuario"})
	}

	return c.JSON(fiber.Map{
		"user_id":     userID,
		"webhook_url": webhookURL,
	})
}
