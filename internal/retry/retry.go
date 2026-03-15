package retry

import (
	"context"
	"math/rand"
	"time"
)

type Config struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Jitter      bool
}

func Do(ctx context.Context, cfg Config, fn func() error) error {
	var err error
	delay := cfg.BaseDelay
	for i := 0; i < cfg.MaxAttempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i == cfg.MaxAttempts-1 {
			break
		}
		wait := delay
		if cfg.Jitter {
			wait += time.Duration(rand.Int63n(int64(delay) + 1))
		}
		if cfg.MaxDelay > 0 && wait > cfg.MaxDelay {
			wait = cfg.MaxDelay
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		delay *= 2
	}
	return err
}
