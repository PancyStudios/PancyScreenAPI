package api

import (
	"context"
	"fmt"
	"time"

	"github.com/PancyStudios/PancyScreenShots/pkg/browser"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/gofiber/fiber/v2"
)

type IAFetchRequest struct {
	Prompt string `json:"prompt"`
}

// HandleIAFetch intercepts the network to execute an AI request via browser fetch
func HandleIAFetch(c *fiber.Ctx) error {
	var req IAFetchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "JSON inválido"})
	}

	if req.Prompt == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Se requiere un prompt"})
	}

	tabCtx, cancelTab := browser.NewTabContext(false)
	defer cancelTab()

	ctx, cancelTimeout := context.WithTimeout(tabCtx, 60*time.Second)
	defer cancelTimeout()

	// Interceptar peticiones de red
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventResponseReceived:
			// Aquí interceptas cuando llega la respuesta.
			// e.Response.URL contendrá la URL interceptada
			// Para atrapar el body, se debe usar network.GetResponseBody(e.RequestId) en una tarea separada.
			_ = e
		}
	})

	// Script JS para inyectar en la consola y simular clic
	jsScript := fmt.Sprintf(`
		(async () => {
			// 1. Encontrar el cuadro de texto
			let ta = document.querySelector("textarea");
			if (!ta) return "Error: No se encontró el textarea";
			
			// Cambiamos el valor nativamente para que React lo detecte
			let nativeInputValueSetter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, "value").set;
			nativeInputValueSetter.call(ta, "%s");
			ta.dispatchEvent(new Event('input', { bubbles: true }));

			// 2. Encontrar el botón de generar
			let buttons = Array.from(document.querySelectorAll("button"));
			let btn = buttons.find(b => 
				b.innerText.toLowerCase().includes("draw") || 
				b.innerText.toLowerCase().includes("dibuj") || 
				b.innerText.toLowerCase().includes("gener")
			);
			
			if (!btn) {
				// Fallback por si el botón no tiene texto pero tiene un id o clase específica
				btn = document.querySelector("#generate-btn") || document.querySelector("button[type='submit']");
			}
			if (!btn) return "Error: No se encontró el botón de generar";
			
			btn.click();

			// 3. Observar y esperar la imagen resultante
			return new Promise((resolve) => {
				let attempts = 0;
				let interval = setInterval(() => {
					attempts++;
					// Buscar imágenes que hayan sido generadas (dominio img.craiyon.com)
					let imgs = Array.from(document.querySelectorAll("img")).filter(img => img.src && img.src.includes("img.craiyon.com"));
					if (imgs.length > 0) {
						clearInterval(interval);
						resolve(imgs[0].src);
					}
					if (attempts > 120) { // Timeout de 60 segundos (120 * 500ms)
						clearInterval(interval);
						resolve("Error: Timeout esperando la imagen");
					}
				}, 500);
			});
		})();
	`, req.Prompt)

	var imageResult string

	// Navegamos a Craiyon para que el navegador pase el chequeo TLS/Cookies de Cloudflare
	err := chromedp.Run(ctx,
		chromedp.Navigate("https://www.craiyon.com/"),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Evaluate(jsScript, &imageResult, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Error ejecutando fetch de Craiyon: " + err.Error()})
	}

	// imageResult contendrá la URL de la imagen generada
	return c.JSON(fiber.Map{
		"image_url": imageResult,
	})
}
