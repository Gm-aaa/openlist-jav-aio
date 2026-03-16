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
		client:    &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAIProvider) Translate(ctx context.Context, srt, targetLang string) (string, error) {
	blocks := SplitSRT(srt)
	if len(blocks) == 0 {
		return srt, nil
	}
	chunks := len(ChunkBlocks(blocks, batchSize))
	p.log.Debug("translating", "blocks", len(blocks), "chunks", chunks, "concurrency", llmConcurrency, "lang", targetLang)

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

// systemPrompt is the fixed instruction used as a system message.
// Separated from user content to enable OpenAI's automatic prompt caching.
const systemPrompt = "You are a subtitle translator. Translate each numbered line to the target language. " +
	"Return ONLY the translated numbered lines in the same format. Do not add explanations."

func (p *OpenAIProvider) translateChunk(ctx context.Context, chunk []SRTBlock, lang string) ([]string, error) {
	// Send only the text lines (no timecodes) to reduce tokens by ~40-60%.
	payload := buildTextOnlyPayload(chunk)
	userMsg := fmt.Sprintf("Translate to %s:\n\n%s", lang, payload)

	reqBody := map[string]any{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMsg},
		},
		"temperature": 0,
	}
	if p.maxTokens > 0 {
		reqBody["max_tokens"] = p.maxTokens
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai API status %d: %s", resp.StatusCode, string(errBody))
	}

	var out struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	lines := parseNumberedLines(out.Choices[0].Message.Content)
	if len(lines) == 0 {
		return nil, fmt.Errorf("LLM returned no translatable lines")
	}
	return lines, nil
}
