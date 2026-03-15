package llm

import "context"

type Provider interface {
	Translate(ctx context.Context, srt string, targetLang string) (string, error)
}

const batchSize = 50
