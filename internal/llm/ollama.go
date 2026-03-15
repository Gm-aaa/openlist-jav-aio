package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	chunks := ChunkBlocks(blocks, batchSize)
	var translated []SRTBlock

	for i, chunk := range chunks {
		p.log.Debug("translating chunk via ollama", "chunk", i+1, "of", len(chunks))
		input := JoinSRT(chunk)
		result, err := p.translateChunk(ctx, input, targetLang)
		if err != nil {
			return "", fmt.Errorf("chunk %d: %w", i, err)
		}
		translated = append(translated, SplitSRT(result)...)
	}
	return JoinSRT(translated), nil
}

func (p *OllamaProvider) translateChunk(ctx context.Context, srt, lang string) (string, error) {
	prompt := fmt.Sprintf(
		"Translate the following SRT subtitle text to %s. "+
			"Preserve all index numbers and timecodes. Only translate dialogue. Return SRT format.\n\n%s", lang, srt)

	body, _ := json.Marshal(map[string]any{
		"model":  p.model,
		"prompt": prompt,
		"stream": false,
	})

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/api/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var out struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Response, nil
}
