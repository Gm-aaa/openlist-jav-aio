package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openlist-jav-aio/jav-aio/internal/retry"
)

func TestDo_SucceedsFirstTry(t *testing.T) {
	calls := 0
	err := retry.Do(context.Background(), retry.Config{MaxAttempts: 3}, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestDo_RetriesAndFails(t *testing.T) {
	calls := 0
	cfg := retry.Config{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}
	err := retry.Do(context.Background(), cfg, func() error {
		calls++
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}
