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
	"time"

	"github.com/PancyStudios/PancyScreenShots/pkg/browser"
	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
)

type Job struct {
	URL        string
	IsSFWOnly  bool
	ResultChan chan JobResult
}

type JobResult struct {
	Image []byte
	Error error
}

var jobQueue chan Job

// InitWorkerPool initializes the worker pool with the given number of workers.
func InitWorkerPool(numWorkers int) {
	jobQueue = make(chan Job, 100) // Allow up to 100 pending jobs
	for i := 0; i < numWorkers; i++ {
		go worker(i)
	}
	log.Printf("Worker pool initialized with %d workers", numWorkers)
}

// AddJob queues a new screenshot job.
func AddJob(job Job) {
	jobQueue <- job
}

func worker(id int) {
	for job := range jobQueue {
		processJob(job)
	}
}

func processJob(job Job) {
	tabCtx, cancelTab := browser.NewTabContext(job.IsSFWOnly)
	defer cancelTab()

	// Timeout to prevent hanging
	ctx, cancelTimeout := context.WithTimeout(tabCtx, 30*time.Second)
	defer cancelTimeout()

	var buf []byte
	err := chromedp.Run(ctx,
		cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorDeny),
		chromedp.Navigate(job.URL),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.CaptureScreenshot(&buf),
	)

	if err != nil {
		if strings.Contains(err.Error(), "ERR_NAME_NOT_RESOLVED") {
			job.ResultChan <- JobResult{Error: errors.New("nsfw_content_detected_by_dns")}
			return
		}
		job.ResultChan <- JobResult{Error: fmt.Errorf("error capturando la pantalla: %w", err)}
		return
	}

	// NSFW validation for SFW routes
	if job.IsSFWOnly {
		isNSFW, err := checkNSFW(buf)
		if err != nil {
			log.Printf("Error checking NSFW: %v", err)
			job.ResultChan <- JobResult{Error: fmt.Errorf("error verificando NSFW: %w", err)}
			return
		}
		if isNSFW {
			job.ResultChan <- JobResult{Error: errors.New("nsfw_content_detected")}
			return
		}
	}

	job.ResultChan <- JobResult{Image: buf, Error: nil}
}

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
	// Send the raw image bytes to the local node.js microservice
	resp, err := http.Post("http://127.0.0.1:3001/check", "application/octet-stream", bytes.NewReader(image))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("detector returned status code: %d", resp.StatusCode)
	}

	var nsfwRes nsfwResponse
	if err := json.NewDecoder(resp.Body).Decode(&nsfwRes); err != nil {
		return false, err
	}

	if nsfwRes.Error != "" {
		return false, fmt.Errorf("detector error: %s", nsfwRes.Error)
	}

	log.Println("=== Reporte de IA (NSFW) ===")
	for _, p := range nsfwRes.Predictions {
		log.Printf(" -> %s: %.2f%%", p.ClassName, p.Probability*100)
	}
	log.Println("============================")

	return nsfwRes.IsNSFW, nil
}
