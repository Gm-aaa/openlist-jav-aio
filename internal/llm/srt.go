package llm

import (
	"context"
	"fmt"
	"regexp"
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

// stripMarkdownFences removes common markdown code fences that LLMs wrap
// around their output (e.g. ```srt\n...\n```).
var mdFenceRe = regexp.MustCompile("(?s)^\\s*```[a-zA-Z]*\\n(.*)\\n```\\s*$")

func stripMarkdownFences(s string) string {
	if m := mdFenceRe.FindStringSubmatch(s); len(m) == 2 {
		return m[1]
	}
	return s
}

// translateChunksConcurrent splits blocks into chunks and translates them
// concurrently using translateFn, up to concurrency parallel requests.
// Results are reassembled in original order.
func translateChunksConcurrent(
	ctx context.Context,
	blocks []SRTBlock,
	chunkSize int,
	concurrency int,
	translateFn func(ctx context.Context, chunk []SRTBlock) ([]string, error),
) ([]SRTBlock, error) {
	chunks := ChunkBlocks(blocks, chunkSize)
	results := make([][]SRTBlock, len(chunks))
	errs := make([]error, len(chunks))

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, chunk := range chunks {
		wg.Add(1)
		go func(i int, chunk []SRTBlock) {
			defer wg.Done()

			// Acquire semaphore with context awareness to avoid blocking
			// indefinitely when ctx is cancelled.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			defer func() { <-sem }()

			texts, err := translateFn(ctx, chunk)
			if err != nil {
				errs[i] = fmt.Errorf("chunk %d: %w", i, err)
				return
			}

			// Zip translated texts back onto original blocks.
			translated := make([]SRTBlock, len(chunk))
			for j := range chunk {
				translated[j] = SRTBlock{
					Index:    chunk[j].Index,
					Timecode: chunk[j].Timecode,
				}
				if j < len(texts) {
					translated[j].Text = texts[j]
				} else {
					// LLM returned fewer lines than expected — keep original.
					translated[j].Text = chunk[j].Text
				}
			}
			results[i] = translated
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

// buildTextOnlyPayload extracts just the dialogue text from blocks as numbered lines.
// Returns the payload string and the count of lines.
func buildTextOnlyPayload(blocks []SRTBlock) string {
	var sb strings.Builder
	for i, b := range blocks {
		fmt.Fprintf(&sb, "%d: %s\n", i+1, b.Text)
	}
	return sb.String()
}

// parseNumberedLines parses "1: text\n2: text\n..." back into a slice of strings.
func parseNumberedLines(output string) []string {
	output = strings.TrimSpace(stripMarkdownFences(output))
	lines := strings.Split(output, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip "N: " prefix if present.
		if idx := strings.Index(line, ": "); idx >= 0 && idx <= 5 {
			// Verify prefix is numeric.
			prefix := strings.TrimSpace(line[:idx])
			allDigits := true
			for _, c := range prefix {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				line = line[idx+2:]
			}
		}
		result = append(result, line)
	}
	return result
}
