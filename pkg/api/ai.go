package api

import (
	"github.com/PancyStudios/PancyScreenShots/pkg/metrics"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/proxy"
)

// HandleAIToxicity proxies the toxicity analysis to the Node.js microservice.
func HandleAIToxicity(c *fiber.Ctx) error {
	metrics.IncrAIRequests("toxicity")
	c.Request().Header.Set("Connection", "close")
	return proxy.Do(c, "http://127.0.0.1:3001/analyze/toxicity")
}

// HandleAIScam proxies the anti-scam URL analysis to the Node.js microservice.
func HandleAIScam(c *fiber.Ctx) error {
	metrics.IncrAIRequests("scam")
	c.Request().Header.Set("Connection", "close")
	return proxy.Do(c, "http://127.0.0.1:3001/analyze/scam")
}

// HandleAINSFW proxies raw image NSFW classification to the Node.js microservice.
func HandleAINSFW(c *fiber.Ctx) error {
	metrics.IncrAIRequests("nsfw")
	c.Request().Header.Set("Connection", "close")
	return proxy.Do(c, "http://127.0.0.1:3001/check")
}

// HandleAIOCR proxies an image to the Tesseract OCR endpoint in the Node.js microservice.
// Expects raw image bytes (application/octet-stream).
func HandleAIOCR(c *fiber.Ctx) error {
	metrics.IncrAIRequests("ocr")
	c.Request().Header.Set("Connection", "close")
	return proxy.Do(c, "http://127.0.0.1:3001/analyze/ocr")
}

// HandleAISpam proxies text to the spam detection heuristic in the Node.js microservice.
// Expects JSON body: {"text": "..."}
func HandleAISpam(c *fiber.Ctx) error {
	metrics.IncrAIRequests("spam")
	c.Request().Header.Set("Connection", "close")
	return proxy.Do(c, "http://127.0.0.1:3001/analyze/spam")
}

// HandleAIScamTrain proxies new training examples to the scam model retraining endpoint.
// Expects JSON body: {"examples": [{"url": "...", "label": 0|1}]}
func HandleAIScamTrain(c *fiber.Ctx) error {
	c.Request().Header.Set("Connection", "close")
	return proxy.Do(c, "http://127.0.0.1:3001/ai/scam/train")
}
