package state_test

import (
	"testing"
	"github.com/openlist-jav-aio/jav-aio/internal/state"
)

func TestDB_CreateAndGet(t *testing.T) {
	db, err := state.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rec := &state.Record{
		OpenListPath: "/jav/ABC-123.mp4",
		JavID:        "ABC-123",
	}
	if err := db.Upsert(rec); err != nil {
		t.Fatal(err)
	}

	got, err := db.Get("/jav/ABC-123.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if got.JavID != "ABC-123" {
		t.Errorf("expected ABC-123, got %s", got.JavID)
	}
}

func TestDB_ListIncomplete(t *testing.T) {
	db, _ := state.Open(":memory:")
	defer db.Close()

	db.Upsert(&state.Record{OpenListPath: "/a.mp4", JavID: "AAA-001"})
	db.Upsert(&state.Record{OpenListPath: "/b.mp4", JavID: "BBB-002",
		ScrapeDone: true, StrmDone: true, SubtitleDone: true, TranslateDone: true})

	// All steps enabled — /a.mp4 is incomplete
	incomplete, err := db.ListIncomplete(state.EnabledSteps{Scrape: true, STRM: true, Subtitle: true, Translate: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(incomplete) != 1 {
		t.Errorf("expected 1 incomplete, got %d", len(incomplete))
	}
	if incomplete[0].OpenListPath != "/a.mp4" {
		t.Errorf("unexpected path: %s", incomplete[0].OpenListPath)
	}
}

func TestDB_ListIncomplete_DisabledSteps(t *testing.T) {
	db, _ := state.Open(":memory:")
	defer db.Close()

	// translate_done=false but translate step disabled — should not count as incomplete
	db.Upsert(&state.Record{OpenListPath: "/c.mp4", JavID: "CCC-003",
		ScrapeDone: true, StrmDone: true, SubtitleDone: true, TranslateDone: false})

	incomplete, err := db.ListIncomplete(state.EnabledSteps{Scrape: true, STRM: true, Subtitle: true, Translate: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(incomplete) != 0 {
		t.Errorf("expected 0 incomplete with translate disabled, got %d", len(incomplete))
	}
}
