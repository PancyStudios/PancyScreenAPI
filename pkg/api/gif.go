package api

import (
	"bytes"
	"context"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"

	"github.com/PancyStudios/PancyScreenShots/pkg/browser"
	"github.com/PancyStudios/PancyScreenShots/pkg/metrics"
	"github.com/gofiber/fiber/v2"
)

// GIFRequest holds the request body for animated GIF generation.
type GIFRequest struct {
	URL        string `json:"url"`
	UserID     string `json:"user_id"`
	Frames     int    `json:"frames"`      // number of frames (default 5, max 20)
	IntervalMS int    `json:"interval_ms"` // ms between frames (default 1000)
}

// HandleGIF takes multiple screenshots at timed intervals and encodes them as an animated GIF.
// Route: POST /api/private/screenshot/gif
func HandleGIF(c *fiber.Ctx) error {
	start := time.Now()

	var req GIFRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "JSON inválido"})
	}
	if req.URL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Se requiere una URL"})
	}

	if err := validateSecurity(req.URL); err != nil {
		metrics.IncrErrors("gif", "ssrf")
		return c.Status(423).JSON(fiber.Map{"error": err.Error()})
	}

	// Sanitize and apply defaults
	frames := req.Frames
	if frames <= 0 || frames > 20 {
		frames = 5
	}
	intervalMS := req.IntervalMS
	if intervalMS < 100 {
		intervalMS = 1000
	}

	// Use the NSFW browser tab (no DNS filtering needed for GIF)
	tabCtx, cancelTab := browser.NewTabContext(false)
	defer cancelTab()

	totalTimeout := time.Duration(frames*intervalMS)*time.Millisecond + 40*time.Second
	ctx, cancelTimeout := context.WithTimeout(tabCtx, totalTimeout)
	defer cancelTimeout()

	// Navigate and wait for page to be ready
	if err := chromedp.Run(ctx,
		cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorDeny),
		chromedp.Navigate(req.URL),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
	); err != nil {
		metrics.IncrErrors("gif", "navigation")
		return c.Status(500).JSON(fiber.Map{"error": "Error navegando a la URL: " + err.Error()})
	}

	// Capture frames
	var gifFrames []*image.Paletted
	var delays []int

	for i := 0; i < frames; i++ {
		var buf []byte
		if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&buf)); err != nil {
			break
		}

		frame, err := pngToPaletted(buf)
		if err != nil {
			break
		}

		gifFrames = append(gifFrames, frame)
		delays = append(delays, intervalMS/10) // centiseconds

		if i < frames-1 {
			_ = chromedp.Run(ctx, chromedp.Sleep(time.Duration(intervalMS)*time.Millisecond))
		}
	}

	if len(gifFrames) == 0 {
		metrics.IncrErrors("gif", "no_frames")
		return c.Status(500).JSON(fiber.Map{"error": "No se pudieron capturar frames"})
	}

	// Encode as animated GIF
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{Image: gifFrames, Delay: delays}); err != nil {
		metrics.IncrErrors("gif", "encode")
		return c.Status(500).JSON(fiber.Map{"error": "Error codificando GIF"})
	}

	metrics.IncrScreenshots("gif", false)
	metrics.ObserveDuration("gif", time.Since(start).Seconds())

	c.Set("Content-Type", "image/gif")
	return c.Send(buf.Bytes())
}

// pngToPaletted converts raw PNG bytes to a 256-color paletted image using
// the Plan9 palette and Floyd-Steinberg dithering for GIF encoding.
func pngToPaletted(pngData []byte) (*image.Paletted, error) {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	paletted := image.NewPaletted(bounds, palette.Plan9)
	draw.FloydSteinberg.Draw(paletted, bounds, img, image.Point{})
	return paletted, nil
}
