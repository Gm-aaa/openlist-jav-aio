package llm

import "context"

type Provider interface {
	Translate(ctx context.Context, srt string, targetLang string) (string, error)
}

// batchSize is the maximum number of SRT blocks per LLM request chunk.
// Chunks are sent concurrently (see llmConcurrency), so total time ≈ time of
// one chunk rather than batchSize × chunks.
// 50 blocks ≈ 500–1000 input tokens + similar output, well within any model limit.
const batchSize = 50

// llmConcurrency is the maximum number of concurrent LLM translation requests.
const llmConcurrency = 10
