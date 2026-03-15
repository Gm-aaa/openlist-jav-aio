package webhook_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlist-jav-aio/jav-aio/internal/webhook"
)

func sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWebhook_ValidOpenListPayload(t *testing.T) {
	queued := make(chan string, 1)
	srv := webhook.NewServer("test-secret", func(path, javID string) {
		queued <- path
	}, nil)

	payload := map[string]any{"source": "openlist", "event": "file.created", "path": "/jav/ABC-123.mp4"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign([]byte("test-secret"), body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
	select {
	case p := <-queued:
		if p != "/jav/ABC-123.mp4" {
			t.Errorf("unexpected path: %s", p)
		}
	default:
		t.Error("expected item in queue")
	}
}

func TestWebhook_InvalidSignature(t *testing.T) {
	srv := webhook.NewServer("test-secret", func(_, _ string) {}, nil)
	body := []byte(`{"source":"openlist","event":"file.created","path":"/jav/X.mp4"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=badhash")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
