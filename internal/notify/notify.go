package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Event is the JSON payload sent to the notify URL.
type Event struct {
	Event     string `json:"event"`
	JavID     string `json:"jav_id"`
	Path      string `json:"path"`
	SRTPath   string `json:"srt_path"`
	Timestamp string `json:"timestamp"`
}

// Notifier sends outgoing webhook notifications.
type Notifier struct {
	url     string
	headers map[string]string
	log     *slog.Logger
	client  *http.Client
}

func New(url string, headers map[string]string, log *slog.Logger) *Notifier {
	if log == nil {
		log = slog.Default()
	}
	return &Notifier{
		url:     url,
		headers: headers,
		log:     log,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

// Send fires a translate_done notification. Errors are logged but not returned,
// so a broken webhook endpoint never fails the pipeline.
func (n *Notifier) Send(ctx context.Context, javID, path, srtPath string) {
	evt := Event{
		Event:     "translate_done",
		JavID:     javID,
		Path:      path,
		SRTPath:   srtPath,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	body, _ := json.Marshal(evt)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		n.log.Warn("notify: build request failed", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range n.headers {
		req.Header.Set(k, v)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		n.log.Warn("notify: request failed", "url", n.url, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		n.log.Warn("notify: non-2xx response", "url", n.url, "status", resp.StatusCode)
		return
	}
	n.log.Info("notify: sent", "jav_id", javID, "status", resp.StatusCode)
}
