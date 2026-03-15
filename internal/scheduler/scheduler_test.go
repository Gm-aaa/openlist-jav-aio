package scheduler_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openlist-jav-aio/jav-aio/internal/scheduler"
)

func TestScheduler_RunsOnInterval(t *testing.T) {
	var count atomic.Int32
	s := scheduler.New(50*time.Millisecond, func(ctx context.Context) {
		count.Add(1)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Millisecond)
	defer cancel()
	s.Start(ctx)
	<-ctx.Done()
	if count.Load() < 2 {
		t.Errorf("expected at least 2 runs, got %d", count.Load())
	}
}
