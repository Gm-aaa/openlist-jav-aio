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

type OpenAIProvider struct {
	baseURL string
	apiKey  string
	model   string
	log     *slog.Logger
	client  *http.Client
}

func NewOpenAIProvider(baseURL, apiKey, model string, log *slog.Logger) *OpenAIProvider {
	if log == nil {
		log = slog.Default()
	}
	return &OpenAIProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey, model: model, log: log,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAIProvider) Translate(ctx context.Context, srt, targetLang string) (string, error) {
	blocks := SplitSRT(srt)
	chunks := ChunkBlocks(blocks, batchSize)
	var translated []SRTBlock

	for i, chunk := range chunks {
		p.log.Debug("translating chunk", "chunk", i+1, "of", len(chunks), "lang", targetLang)
		input := JoinSRT(chunk)
		result, err := p.translateChunk(ctx, input, targetLang)
		if err != nil {
			return "", fmt.Errorf("chunk %d: %w", i, err)
		}
		translated = append(translated, SplitSRT(result)...)
	}
	return JoinSRT(translated), nil
}

func (p *OpenAIProvider) translateChunk(ctx context.Context, srt, lang string) (string, error) {
	prompt := fmt.Sprintf(
		"Translate the following SRT subtitle text lines to %s. "+
			"Keep all index numbers and timecodes exactly unchanged. "+
			"Only translate the dialogue text lines. Return valid SRT format.\n\n%s", lang, srt)

	body, _ := json.Marshal(map[string]any{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("openai API status %d", resp.StatusCode)
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
