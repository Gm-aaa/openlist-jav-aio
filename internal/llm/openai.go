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

type OpenAIProvider struct {
	baseURL   string
	apiKey    string
	model     string
	maxTokens int // 0 = omit from request (use API default)
	log       *slog.Logger
	client    *http.Client
}

func NewOpenAIProvider(baseURL, apiKey, model string, maxTokens int, log *slog.Logger) *OpenAIProvider {
	if log == nil {
		log = slog.Default()
	}
	return &OpenAIProvider{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		model:     model,
		maxTokens: maxTokens,
		log:       log,
		client:    &http.Client{Timeout: 600 * time.Second},
	}
}

func (p *OpenAIProvider) Translate(ctx context.Context, srt, targetLang string) (string, error) {
	blocks := SplitSRT(srt)
	if len(blocks) == 0 {
		return srt, nil
	}
	chunks := len(ChunkBlocks(blocks, batchSize))
	p.log.Debug("translating", "blocks", len(blocks), "chunks", chunks, "concurrency", llmConcurrency, "lang", targetLang)

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

func (p *OpenAIProvider) translateChunk(ctx context.Context, srt, lang string) (string, error) {
	prompt := fmt.Sprintf(
		"Translate the following SRT subtitle text lines to %s. "+
			"Keep all index numbers and timecodes exactly unchanged. "+
			"Only translate the dialogue text lines. Return valid SRT format.\n\n%s", lang, srt)

	reqBody := map[string]any{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	if p.maxTokens > 0 {
		reqBody["max_tokens"] = p.maxTokens
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai API status %d: %s", resp.StatusCode, string(errBody))
	}

	var out struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return out.Choices[0].Message.Content, nil
}
