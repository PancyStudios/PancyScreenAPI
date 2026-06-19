package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/PancyStudios/PancyScreenShots/pkg/browser"
	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// JobType identifies the kind of work a Job requests.
type JobType string

const (
	JobTypeScreenshot JobType = "screenshot"
	JobTypePDF        JobType = "pdf"
	JobTypeElement    JobType = "element"
)

// Job holds all parameters for a single unit of browser work.
type Job struct {
	Type       JobType
	URL        string
	IsSFWOnly  bool
	Width      int    // Viewport width  (0 = default 1920)
	Height     int    // Viewport height (0 = default 1080)
	FullPage   bool   // Capture full scrollable page
	Selector   string // CSS selector for JobTypeElement
	ResultChan chan JobResult
}

// JobResult carries the raw bytes produced by a Job, or an error.
type JobResult struct {
	Data  []byte
	Error error
}

// ───────────────────────────────────────────────────────────────────────────────
// Stats
// ───────────────────────────────────────────────────────────────────────────────

var (
	activeWorkers int64
	pendingJobs   int64
	totalWorkers  int
	queueCapacity int
)

// QueueStats is returned by GetStats for the /queue/status endpoint.
type QueueStats struct {
	PendingJobs   int64 `json:"pending_jobs"`
	ActiveWorkers int64 `json:"active_workers"`
	TotalWorkers  int   `json:"total_workers"`
	QueueCapacity int   `json:"queue_capacity"`
}

// GetStats returns a snapshot of the worker pool state.
func GetStats() QueueStats {
	return QueueStats{
		PendingJobs:   atomic.LoadInt64(&pendingJobs),
		ActiveWorkers: atomic.LoadInt64(&activeWorkers),
		TotalWorkers:  totalWorkers,
		QueueCapacity: queueCapacity,
	}
}

// ───────────────────────────────────────────────────────────────────────────────
// Pool
// ───────────────────────────────────────────────────────────────────────────────

var jobQueue chan Job

// InitWorkerPool starts numWorkers goroutines that drain the job queue.
func InitWorkerPool(numWorkers int) {
	totalWorkers = numWorkers
	queueCapacity = 100
	jobQueue = make(chan Job, queueCapacity)
	for i := 0; i < numWorkers; i++ {
		go worker(i)
	}
	log.Printf("[Queue] Worker pool iniciado con %d workers", numWorkers)
}

// AddJob enqueues a new job. Increments the pendingJobs counter.
func AddJob(job Job) {
	atomic.AddInt64(&pendingJobs, 1)
	jobQueue <- job
}

func worker(id int) {
	for job := range jobQueue {
		atomic.AddInt64(&pendingJobs, -1)
		atomic.AddInt64(&activeWorkers, 1)
		processJob(job)
		atomic.AddInt64(&activeWorkers, -1)
	}
}

// ───────────────────────────────────────────────────────────────────────────────
// Job Dispatcher
// ───────────────────────────────────────────────────────────────────────────────

func processJob(job Job) {
	switch job.Type {
	case JobTypePDF:
		processPDFJob(job)
	case JobTypeElement:
		processElementJob(job)
	default:
		processScreenshotJob(job)
	}
}

// ───────────────────────────────────────────────────────────────────────────────
// Screenshot Job
// ───────────────────────────────────────────────────────────────────────────────

func processScreenshotJob(job Job) {
	tabCtx, cancelTab := browser.NewTabContext(job.IsSFWOnly)
	defer cancelTab()

	ctx, cancelTimeout := context.WithTimeout(tabCtx, 30*time.Second)
	defer cancelTimeout()

	actions := []chromedp.Action{
		cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorDeny),
	}

	// Apply custom viewport if specified
	if job.Width > 0 || job.Height > 0 {
		w, h := int64(job.Width), int64(job.Height)
		if w == 0 {
			w = 1920
		}
		if h == 0 {
			h = 1080
		}
		actions = append(actions, chromedp.EmulateViewport(w, h))
	}

	actions = append(actions,
		chromedp.Navigate(job.URL),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
	)

	var buf []byte
	if job.FullPage {
		actions = append(actions, chromedp.FullScreenshot(&buf, 90))
	} else {
		actions = append(actions, chromedp.CaptureScreenshot(&buf))
	}

	if err := chromedp.Run(ctx, actions...); err != nil {
		if strings.Contains(err.Error(), "ERR_NAME_NOT_RESOLVED") {
			job.ResultChan <- JobResult{Error: errors.New("nsfw_content_detected_by_dns")}
			return
		}
		job.ResultChan <- JobResult{Error: fmt.Errorf("error capturando pantalla: %w", err)}
		return
	}

	if job.IsSFWOnly {
		isNSFW, err := checkNSFW(buf)
		if err != nil {
			log.Printf("[Worker] Error comprobando NSFW: %v", err)
			job.ResultChan <- JobResult{Error: fmt.Errorf("error verificando NSFW: %w", err)}
			return
		}
		if isNSFW {
			job.ResultChan <- JobResult{Error: errors.New("nsfw_content_detected")}
			return
		}
	}

	job.ResultChan <- JobResult{Data: buf}
}

// ───────────────────────────────────────────────────────────────────────────────
// PDF Job
// ───────────────────────────────────────────────────────────────────────────────

func processPDFJob(job Job) {
	tabCtx, cancelTab := browser.NewTabContext(false)
	defer cancelTab()

	ctx, cancelTimeout := context.WithTimeout(tabCtx, 30*time.Second)
	defer cancelTimeout()

	var pdfBuf []byte
	err := chromedp.Run(ctx,
		cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorDeny),
		chromedp.Navigate(job.URL),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.ActionFunc(func(ctx context.Context) error {
			buf, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				Do(ctx)
			if err != nil {
				return err
			}
			pdfBuf = buf
			return nil
		}),
	)

	if err != nil {
		job.ResultChan <- JobResult{Error: fmt.Errorf("error generando PDF: %w", err)}
		return
	}
	job.ResultChan <- JobResult{Data: pdfBuf}
}

// ───────────────────────────────────────────────────────────────────────────────
// Element Screenshot Job
// ───────────────────────────────────────────────────────────────────────────────

func processElementJob(job Job) {
	tabCtx, cancelTab := browser.NewTabContext(job.IsSFWOnly)
	defer cancelTab()

	ctx, cancelTimeout := context.WithTimeout(tabCtx, 30*time.Second)
	defer cancelTimeout()

	var buf []byte
	err := chromedp.Run(ctx,
		cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorDeny),
		chromedp.Navigate(job.URL),
		chromedp.WaitVisible(job.Selector, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
		chromedp.Screenshot(job.Selector, &buf, chromedp.ByQuery),
	)

	if err != nil {
		if strings.Contains(err.Error(), "ERR_NAME_NOT_RESOLVED") {
			job.ResultChan <- JobResult{Error: errors.New("nsfw_content_detected_by_dns")}
			return
		}
		job.ResultChan <- JobResult{Error: fmt.Errorf("error capturando elemento '%s': %w", job.Selector, err)}
		return
	}

	job.ResultChan <- JobResult{Data: buf}
}

// ───────────────────────────────────────────────────────────────────────────────
// NSFW checker (calls Node.js microservice)
// ───────────────────────────────────────────────────────────────────────────────

type prediction struct {
	ClassName   string  `json:"className"`
	Probability float64 `json:"probability"`
}

type nsfwResponse struct {
	IsNSFW      bool         `json:"isNSFW"`
	Probability float64      `json:"probability"`
	Predictions []prediction `json:"predictions"`
	Error       string       `json:"error,omitempty"`
}

func checkNSFW(image []byte) (bool, error) {
	resp, err := http.Post("http://127.0.0.1:3001/check", "application/octet-stream", bytes.NewReader(image))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("detector returned status %d", resp.StatusCode)
	}

	var nsfwRes nsfwResponse
	if err := json.NewDecoder(resp.Body).Decode(&nsfwRes); err != nil {
		return false, err
	}

	if nsfwRes.Error != "" {
		return false, fmt.Errorf("detector error: %s", nsfwRes.Error)
	}

	log.Println("=== Reporte IA (NSFW) ===")
	for _, p := range nsfwRes.Predictions {
		log.Printf("  -> %s: %.2f%%", p.ClassName, p.Probability*100)
	}
	log.Println("=========================")

	return nsfwRes.IsNSFW, nil
}
