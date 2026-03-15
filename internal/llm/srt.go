package llm

import "strings"

type SRTBlock struct {
	Index    string
	Timecode string
	Text     string
}

func SplitSRT(content string) []SRTBlock {
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
