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

	translated, err := translateChunksConcurrent(ctx, blocks, batchSize, llmConcurrency,
		func(ctx context.Context, chunk []SRTBlock) ([]string, error) {
			return p.translateChunk(ctx, chunk, targetLang)
		},
	)
	if err != nil {
		return "", err
	}
	return JoinSRT(translated), nil
}

func (p *OllamaProvider) translateChunk(ctx context.Context, chunk []SRTBlock, lang string) ([]string, error) {
	payload := buildTextOnlyPayload(chunk)
	prompt := fmt.Sprintf(
		"Translate each numbered line to %s. Return ONLY the translated numbered lines in the same format.\n\n%s", lang, payload)

	body, err := json.Marshal(map[string]any{
		"model":       p.model,
		"prompt":      prompt,
		"stream":      false,
		"temperature": 0,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama API status %d: %s", resp.StatusCode, string(errBody))
	}

	var out struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	lines := parseNumberedLines(out.Response)
	if len(lines) == 0 {
		return nil, fmt.Errorf("LLM returned no translatable lines")
	}
	return lines, nil
}
