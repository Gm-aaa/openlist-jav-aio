package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type OllamaProvider struct {
	baseURL string
	model   string
	log     *slog.Logger
	client  *http.Client
}

func NewOllamaProvider(baseURL, model string, log *slog.Logger) *OllamaProvider {
	if log == nil {
		log = slog.Default()
	}
	return &OllamaProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model, log: log,
		client: &http.Client{Timeout: 300 * time.Second},
	}
}

func (p *OllamaProvider) Translate(ctx context.Context, srt, targetLang string) (string, error) {
	blocks := SplitSRT(srt)
	if len(blocks) == 0 {
		return srt, nil
	}
	p.log.Debug("translating via ollama", "blocks", len(blocks), "concurrency", llmConcurrency)

	translated, err := translateChunksConcurrent(ctx, blocks, llmConcurrency,
		func(ctx context.Context, chunk string) (string, error) {
			return p.translateChunk(ctx, chunk, targetLang)
		},
	)
	if err != nil {
		return "", err
	}
	return JoinSRT(translated), nil
}

func (p *OllamaProvider) translateChunk(ctx context.Context, srt, lang string) (string, error) {
	prompt := fmt.Sprintf(
		"Translate the following SRT subtitle text to %s. "+
			"Preserve all index numbers and timecodes. Only translate dialogue. Return SRT format.\n\n%s", lang, srt)

	body, err := json.Marshal(map[string]any{
		"model":  p.model,
		"prompt": prompt,
		"stream": false,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama API status %d: %s", resp.StatusCode, string(errBody))
	}

	var out struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Response, nil
}
