package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type SRTBlock struct {
	Index    string
	Timecode string
	Text     string
}

func SplitSRT(content string) []SRTBlock {
	// Normalise CRLF (Windows) and CR (old Mac) to LF so block splitting works
	// regardless of the line endings written by the SRT generator.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	var blocks []SRTBlock
	entries := strings.Split(strings.TrimSpace(content), "\n\n")
	for _, entry := range entries {
		lines := strings.SplitN(strings.TrimSpace(entry), "\n", 3)
		if len(lines) < 3 {
			continue
		}
		blocks = append(blocks, SRTBlock{
			Index:    lines[0],
			Timecode: lines[1],
			Text:     lines[2],
		})
	}
	return blocks
}

func JoinSRT(blocks []SRTBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		sb.WriteString(b.Index + "\n")
		sb.WriteString(b.Timecode + "\n")
		sb.WriteString(b.Text + "\n\n")
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func ChunkBlocks(blocks []SRTBlock, n int) [][]SRTBlock {
	var chunks [][]SRTBlock
	for len(blocks) > 0 {
		end := n
		if end > len(blocks) {
			end = len(blocks)
		}
		chunks = append(chunks, blocks[:end])
		blocks = blocks[end:]
	}
	return chunks
}

// translateChunksConcurrent splits blocks into chunks and translates them
// concurrently using translateFn, up to concurrency parallel requests.
// Results are reassembled in original order.
func translateChunksConcurrent(
	ctx context.Context,
	blocks []SRTBlock,
	concurrency int,
	translateFn func(ctx context.Context, srt string) (string, error),
) ([]SRTBlock, error) {
	chunks := ChunkBlocks(blocks, batchSize)
	results := make([][]SRTBlock, len(chunks))
	errs := make([]error, len(chunks))

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, chunk := range chunks {
		wg.Add(1)
		go func(i int, chunk []SRTBlock) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			out, err := translateFn(ctx, JoinSRT(chunk))
			if err != nil {
				errs[i] = fmt.Errorf("chunk %d: %w", i, err)
				return
			}
			parsed := SplitSRT(out)
			if len(parsed) == 0 {
				errs[i] = fmt.Errorf("chunk %d: translation returned no SRT blocks (LLM output may be malformed)", i)
				return
			}
			results[i] = parsed
		}(i, chunk)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	var translated []SRTBlock
	for _, r := range results {
		translated = append(translated, r...)
	}
	return translated, nil
}
