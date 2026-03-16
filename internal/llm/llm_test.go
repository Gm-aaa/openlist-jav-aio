package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlist-jav-aio/jav-aio/internal/llm"
)

func TestSplitSRT(t *testing.T) {
	srt := "1\n00:00:01,000 --> 00:00:02,000\nHello\n\n2\n00:00:03,000 --> 00:00:04,000\nWorld\n"
	blocks := llm.SplitSRT(srt)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
}

func TestOpenAIProvider_Translate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "1: 你好"}},
			},
		})
	}))
	defer srv.Close()

	p := llm.NewOpenAIProvider(srv.URL, "test-key", "gpt-4o-mini", 0, nil)
	result, err := p.Translate(context.Background(),
		"1\n00:00:01,000 --> 00:00:02,000\nHello\n", "zh")
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty translation")
	}
}
