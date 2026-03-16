package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/openlist-jav-aio/jav-aio/internal/id"
	"github.com/openlist-jav-aio/jav-aio/internal/pipeline"
	"github.com/openlist-jav-aio/jav-aio/internal/scheduler"
	"github.com/openlist-jav-aio/jav-aio/internal/state"
	"github.com/openlist-jav-aio/jav-aio/internal/webhook"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the JAV-AIO daemon (scheduler + webhook)",
	RunE:  runDaemon,
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}

func runDaemon(cmd *cobra.Command, args []string) error {
	app, err := buildApp()
	if err != nil {
		return fmt.Errorf("build app: %w", err)
	}
	defer app.DB.Close()

	if err := app.Cfg.Validate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	cfg := app.Cfg
	log := app.Log

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	taskQueue := make(chan pipeline.Task, 100)
	workerDone := make(chan struct{})

	// Dedup set: tracks paths currently in the queue or being processed.
	var inflight sync.Map

	// Start single worker goroutine.
	go func() {
		defer close(workerDone)
		for task := range taskQueue {
			if err := app.Pipeline.Run(ctx, task); err != nil {
				log.Error("pipeline error", "id", task.JavID, "error", err)
			}
			inflight.Delete(task.OpenListPath)
		}
	}()

	// trySend sends a task to the queue, respecting context cancellation to avoid
	// sending on a closed channel. Returns false if ctx is cancelled or queue is full.
	trySend := func(task pipeline.Task) bool {
		select {
		case <-ctx.Done():
			return false
		case taskQueue <- task:
			return true
		default:
			log.Warn("task queue full, dropping task", "id", task.JavID, "path", task.OpenListPath)
			return false
		}
	}

	// enqueue builds and sends a Task into taskQueue.
	enqueue := func(olPath, javID string) {
		// Bail early if shutting down.
		if ctx.Err() != nil {
			return
		}

		if olPath == "" && javID != "" {
			// Search scan paths for a matching file.
			for _, scanPath := range cfg.OpenList.ScanPaths {
				files, err := app.OL.ListFiles(ctx, scanPath, cfg.OpenList.ScanExtensions)
				if err != nil {
					log.Warn("enqueue: list files failed", "path", scanPath, "error", err)
					continue
				}
				for _, f := range files {
					if extractedID, ok := id.Extract(f.Name); ok && extractedID == javID {
						olPath = f.Path
						break
					}
				}
				if olPath != "" {
					break
				}
			}
		}

		if olPath == "" {
			log.Warn("enqueue: could not resolve openlist path", "jav_id", javID)
			return
		}

		extractedID, ok := id.Extract(olPath)
		if !ok && javID == "" {
			log.Warn("enqueue: could not extract JAV ID", "path", olPath)
			return
		}
		if ok {
			javID = extractedID
		}

		// Skip files that have already completed all enabled steps.
		// This avoids unnecessary /api/fs/get calls for every known file on each scan.
		enabledSteps := state.EnabledSteps{
			Scrape:    cfg.Pipeline.Steps.Scrape,
			STRM:      cfg.Pipeline.Steps.STRM,
			Subtitle:  cfg.Pipeline.Steps.Subtitle,
			Translate: cfg.Pipeline.Steps.Translate,
		}
		if app.DB.IsComplete(olPath, enabledSteps) {
			return
		}

		fileURL, err := app.OL.GetFileURL(ctx, olPath, "")
		if err != nil {
			log.Warn("enqueue: get file URL failed", "path", olPath, "error", err)
			return
		}

		outDir := filepath.Join(cfg.Output.BaseDir, javID)
		task := pipeline.Task{
			OpenListPath: olPath,
			JavID:        javID,
			FileURL:      fileURL,
			OutDir:       outDir,
		}

		// Deduplicate: skip if this path is already queued or being processed.
		if _, loaded := inflight.LoadOrStore(olPath, struct{}{}); loaded {
			log.Debug("enqueue: skipping duplicate", "path", olPath)
			return
		}

		if !trySend(task) {
			inflight.Delete(olPath)
		}
	}

	// Track background goroutines to wait for them before closing taskQueue.
	var bgWg sync.WaitGroup

	// Re-enqueue incomplete tasks at startup.
	bgWg.Add(1)
	go func() {
		defer bgWg.Done()
		steps := state.EnabledSteps{
			Scrape:    cfg.Pipeline.Steps.Scrape,
			STRM:      cfg.Pipeline.Steps.STRM,
			Subtitle:  cfg.Pipeline.Steps.Subtitle,
			Translate: cfg.Pipeline.Steps.Translate,
		}
		incomplete, err := app.DB.ListIncomplete(steps)
		if err != nil {
			log.Error("list incomplete tasks", "error", err)
			return
		}
		log.Info("re-enqueueing incomplete tasks", "count", len(incomplete))
		for _, rec := range incomplete {
			if ctx.Err() != nil {
				return
			}
			enqueue(rec.OpenListPath, rec.JavID)
		}
	}()

	// Scheduler: scan all scan paths on interval.
	var httpServer *http.Server

	pollInterval, err := time.ParseDuration(cfg.Pipeline.PollInterval)
	if err != nil || pollInterval <= 0 {
		log.Warn("invalid poll_interval, scheduler disabled", "value", cfg.Pipeline.PollInterval)
	} else {
		scanFn := func(ctx context.Context) {
			for _, scanPath := range cfg.OpenList.ScanPaths {
				if ctx.Err() != nil {
					return
				}
				files, err := app.OL.ListFiles(ctx, scanPath, cfg.OpenList.ScanExtensions)
				if err != nil {
					log.Warn("scan: list files failed", "path", scanPath, "error", err)
					continue
				}
				for _, f := range filterBySize(files, app.MinFileBytes) {
					if ctx.Err() != nil {
						return
					}
					enqueue(f.Path, "")
				}
			}
		}
		sched := scheduler.New(pollInterval, scanFn).WithLogger(log)
		bgWg.Add(1)
		go func() {
			defer bgWg.Done()
			sched.Start(ctx)
		}()
	}

	// Webhook server.
	if cfg.Webhook.Enabled {
		handler := webhook.NewServer(cfg.Webhook.Secret, enqueue, log)
		addr := fmt.Sprintf(":%d", cfg.Webhook.Port)
		httpServer = &http.Server{Addr: addr, Handler: handler}
		go func() {
			log.Info("webhook server listening", "addr", addr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error("webhook server error", "error", err)
			}
		}()
	}

	log.Info("daemon started")
	<-ctx.Done()
	log.Info("daemon shutting down")

	// Graceful HTTP shutdown — stops accepting new webhook requests.
	if httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Warn("http server shutdown error", "error", err)
		}
	}

	// Wait for scheduler and re-enqueue goroutines to finish, then close queue.
	bgWg.Wait()
	close(taskQueue)

	// Wait for worker to drain (up to 5 minutes).
	select {
	case <-workerDone:
		log.Info("worker drained")
	case <-time.After(5 * time.Minute):
		log.Warn("worker drain timeout")
	}

	return nil
}
