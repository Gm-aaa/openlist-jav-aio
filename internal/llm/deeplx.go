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
	"sync"
	"time"
)

// DeepLXProvider translates subtitles using a DeepLX instance.
// Each SRT block is translated in parallel to maximise throughput;
// the concurrency level is capped to avoid overwhelming the server.
type DeepLXProvider struct {
	baseURL    string
	sourceLang string // e.g. "JA"; "" = auto-detect
	log        *slog.Logger
	client     *http.Client
}

// NewDeepLXProvider creates a DeepLXProvider.
//   - baseURL:    DeepLX server, e.g. "http://localhost:1188"
//   - sourceLang: BCP-47 / DeepL language code for the source, e.g. "JA".
//                 Pass "" to let DeepL auto-detect.
func NewDeepLXProvider(baseURL, sourceLang string, log *slog.Logger) *DeepLXProvider {
	if log == nil {
		log = slog.Default()
	}
	return &DeepLXProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		sourceLang: strings.ToUpper(sourceLang),
		log:        log,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

// deeplxTargetLang converts a BCP-47 target language code to the format
// DeepL/DeepLX expects (uppercase, "ZH" for Chinese).
func deeplxTargetLang(lang string) string {
	switch strings.ToLower(lang) {
	case "zh", "zh-cn", "zh-hans":
		return "ZH"
	case "zh-tw", "zh-hant":
		return "ZH-TW"
	case "en":
		return "EN-US"
	default:
		return strings.ToUpper(lang)
	}
}

const deeplxConcurrency = 20 // parallel requests to DeepLX

func (p *DeepLXProvider) Translate(ctx context.Context, srt, targetLang string) (string, error) {
	blocks := SplitSRT(srt)
	if len(blocks) == 0 {
		return srt, nil
	}

	p.log.Debug("translating with deeplx", "blocks", len(blocks), "concurrency", deeplxConcurrency)

	target := deeplxTargetLang(targetLang)
	translated := make([]SRTBlock, len(blocks))
	errs := make([]error, len(blocks))

	sem := make(chan struct{}, deeplxConcurrency)
	var wg sync.WaitGroup

	for i, block := range blocks {
		wg.Add(1)
		go func(i int, b SRTBlock) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			text, err := p.translateText(ctx, b.Text, target)
			if err != nil {
				errs[i] = err
				translated[i] = b // keep original on error
				return
			}
			translated[i] = SRTBlock{Index: b.Index, Timecode: b.Timecode, Text: text}
		}(i, block)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return "", fmt.Errorf("block %d: %w", i, err)
		}
	}
	return JoinSRT(translated), nil
}

func (p *DeepLXProvider) translateText(ctx context.Context, text, targetLang string) (string, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"text":        text,
		"source_lang": p.sourceLang,
		"target_lang": targetLang,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/translate", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("deeplx HTTP %d: %s", resp.StatusCode, string(body))
	}

	var out struct {
		Code int    `json:"code"`
		Data string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Code != 200 {
		return "", fmt.Errorf("deeplx error code %d", out.Code)
	}
	return out.Data, nil
}
