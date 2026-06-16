package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/proxy"
)

// HandleAIToxicity actúa como proxy para el microservicio de Toxicidad de Node.js
func HandleAIToxicity(c *fiber.Ctx) error {
	c.Request().Header.Set("Connection", "close")
	return proxy.Do(c, "http://127.0.0.1:3001/analyze/toxicity")
}

// HandleAIScam actúa como proxy para el microservicio Anti-Scam de Node.js
func HandleAIScam(c *fiber.Ctx) error {
	c.Request().Header.Set("Connection", "close")
	return proxy.Do(c, "http://127.0.0.1:3001/analyze/scam")
}

// HandleAINSFW actúa como proxy para evaluar imágenes directamente en el microservicio de Node.js
func HandleAINSFW(c *fiber.Ctx) error {
	c.Request().Header.Set("Connection", "close")
	return proxy.Do(c, "http://127.0.0.1:3001/check")
}
