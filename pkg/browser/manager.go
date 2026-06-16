package browser

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/chromedp/chromedp"
)

var (
	sfwAllocatorCtx    context.Context
	sfwAllocatorCancel context.CancelFunc
	sfwBrowserCtx      context.Context
	sfwBrowserCancel   context.CancelFunc

	nsfwAllocatorCtx    context.Context
	nsfwAllocatorCancel context.CancelFunc
	nsfwBrowserCtx      context.Context
	nsfwBrowserCancel   context.CancelFunc
)

// InitBrowsers initializes two distinct headless browser instances
func InitBrowsers() {
	isHeadless := os.Getenv("SHOW_BROWSER") != "true"
	// Para forzarlo a verse en local siempre, podemos ponerlo en false temporalmente:
	isHeadless = true

	baseOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", isHeadless),
		chromedp.Flag("disable-gpu", false), // GPU is often needed when not headless
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		chromedp.WindowSize(1920, 1080),
	)

	// --- 1. SFW BROWSER (With CleanBrowsing Family Filter DNS) ---
	// Usamos la configuración nativa de Chrome inyectándola en el Local State del perfil
	sfwProfileDir := "./chrome_profile_sfw"
	if err := applySecureDNS(sfwProfileDir, "https://doh.cleanbrowsing.org/doh/family-filter{?dns}"); err != nil {
		log.Printf("Error aplicando DNS seguro al perfil SFW: %v", err)
	}

	sfwOpts := append(baseOpts,
		chromedp.Flag("remote-debugging-port", "9222"),
		chromedp.Flag("remote-debugging-address", "127.0.0.1"),
		chromedp.UserDataDir(sfwProfileDir),
	)

	sfwAllocatorCtx, sfwAllocatorCancel = chromedp.NewExecAllocator(context.Background(), sfwOpts...)
	sfwBrowserCtx, sfwBrowserCancel = chromedp.NewContext(sfwAllocatorCtx)

	if err := chromedp.Run(sfwBrowserCtx); err != nil {
		log.Fatalf("Error iniciando el navegador SFW: %v", err)
	}
	log.Println("Navegador SFW (Filtro DNS activo) inicializado.")

	// --- 2. NSFW BROWSER (Standard DNS) ---
	nsfwOpts := append(baseOpts,
		chromedp.Flag("remote-debugging-port", "9223"), // Diferente puerto para evitar conflicto
		chromedp.Flag("remote-debugging-address", "127.0.0.1"),
		chromedp.UserDataDir("./chrome_profile_nsfw"),
	)

	nsfwAllocatorCtx, nsfwAllocatorCancel = chromedp.NewExecAllocator(context.Background(), nsfwOpts...)
	nsfwBrowserCtx, nsfwBrowserCancel = chromedp.NewContext(nsfwAllocatorCtx)

	if err := chromedp.Run(nsfwBrowserCtx); err != nil {
		log.Fatalf("Error iniciando el navegador NSFW: %v", err)
	}
	log.Println("Navegador NSFW (Sin filtros) inicializado.")
}

// CloseBrowsers gracefully shuts down both browsers
func CloseBrowsers() {
	if sfwBrowserCancel != nil {
		sfwBrowserCancel()
	}
	if sfwAllocatorCancel != nil {
		sfwAllocatorCancel()
	}
	if nsfwBrowserCancel != nil {
		nsfwBrowserCancel()
	}
	if nsfwAllocatorCancel != nil {
		nsfwAllocatorCancel()
	}
}

// NewTabContext creates a new isolated tab for a specific task depending on the route
func NewTabContext(isSFW bool) (context.Context, context.CancelFunc) {
	if isSFW {
		return chromedp.NewContext(sfwBrowserCtx)
	}
	return chromedp.NewContext(nsfwBrowserCtx)
}

// applySecureDNS inyecta la configuración nativa de Chrome para usar un proveedor DNS seguro en el archivo Local State
func applySecureDNS(profileDir string, template string) error {
	_ = os.MkdirAll(profileDir, 0755)
	localStatePath := profileDir + "/Local State"

	var state map[string]interface{}
	data, err := os.ReadFile(localStatePath)
	if err == nil {
		_ = json.Unmarshal(data, &state)
	}
	if state == nil {
		state = make(map[string]interface{})
	}

	state["dns_over_https"] = map[string]interface{}{
		"mode":      "secure",
		"templates": template,
	}

	newData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(localStatePath, newData, 0644)
}
