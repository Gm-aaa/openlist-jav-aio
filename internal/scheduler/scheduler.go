package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type Scheduler struct {
	interval time.Duration
	fn       func(ctx context.Context)
	log      *slog.Logger
}

func New(interval time.Duration, fn func(ctx context.Context)) *Scheduler {
	return &Scheduler{interval: interval, fn: fn, log: slog.Default()}
}

func (s *Scheduler) WithLogger(log *slog.Logger) *Scheduler {
	s.log = log
	return s
}

// Start runs the scheduler synchronously, blocking until ctx is cancelled.
// The function fn is called immediately, then repeated at each interval.
// Panics in fn are recovered and logged so the scheduler continues running.
func (s *Scheduler) Start(ctx context.Context) {
	s.log.Debug("scheduler started", "interval", s.interval)
	s.safeRun(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.log.Debug("scheduler stopped")
			return
		case <-ticker.C:
			s.log.Debug("scheduler tick")
			s.safeRun(ctx)
		}
	}
}

func (s *Scheduler) safeRun(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("scheduler: panic recovered", "panic", r)
		}
	}()
	s.fn(ctx)
}
