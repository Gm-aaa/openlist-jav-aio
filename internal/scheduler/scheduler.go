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

func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		s.log.Debug("scheduler started", "interval", s.interval)
		s.fn(ctx)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				s.log.Debug("scheduler stopped")
				return
			case <-ticker.C:
				s.log.Debug("scheduler tick")
				s.fn(ctx)
			}
		}
	}()
}
