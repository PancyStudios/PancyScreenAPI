package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	screenshotsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pancyscreen_screenshots_total",
		Help: "Total de capturas procesadas.",
	}, []string{"type", "cached"})

	screenshotErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pancyscreen_screenshot_errors_total",
		Help: "Total de errores al procesar capturas.",
	}, []string{"type", "reason"})

	aiRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pancyscreen_ai_requests_total",
		Help: "Total de peticiones al microservicio de IA.",
	}, []string{"model"})

	cacheHitsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pancyscreen_cache_hits_total",
		Help: "Total de respuestas servidas desde caché.",
	})

	requestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pancyscreen_request_duration_seconds",
		Help:    "Duración de las peticiones en segundos.",
		Buckets: prometheus.DefBuckets,
	}, []string{"type"})
)

// IncrScreenshots increments the screenshot counter.
func IncrScreenshots(screenshotType string, cached bool) {
	cachedStr := "false"
	if cached {
		cachedStr = "true"
	}
	screenshotsTotal.WithLabelValues(screenshotType, cachedStr).Inc()
}

// IncrErrors increments the error counter.
func IncrErrors(screenshotType, reason string) {
	screenshotErrorsTotal.WithLabelValues(screenshotType, reason).Inc()
}

// IncrAIRequests increments the AI request counter.
func IncrAIRequests(model string) {
	aiRequestsTotal.WithLabelValues(model).Inc()
}

// IncrCacheHits increments the cache hit counter.
func IncrCacheHits() {
	cacheHitsTotal.Inc()
}

// ObserveDuration records the request duration.
func ObserveDuration(screenshotType string, duration float64) {
	requestDurationSeconds.WithLabelValues(screenshotType).Observe(duration)
}

// Handler returns the Prometheus HTTP handler for scraping.
func Handler() http.Handler {
	return promhttp.Handler()
}
