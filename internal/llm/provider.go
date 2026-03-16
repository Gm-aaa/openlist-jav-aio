package llm

import "context"

type Provider interface {
	Translate(ctx context.Context, srt string, targetLang string) (string, error)
}

// batchSize is the maximum number of SRT blocks per LLM request chunk.
// 100 blocks ≈ 1000–3000 tokens (text-only mode, no timecodes), well within
// any model's context window. Larger batches = fewer API calls = faster.
const batchSize = 100

// llmConcurrency is the default maximum number of concurrent LLM translation requests.
const llmConcurrency = 10
