package id_test

import (
	"testing"
	"github.com/openlist-jav-aio/jav-aio/internal/id"
)

func TestExtract(t *testing.T) {
	cases := []struct {
		input    string
		expected string
		ok       bool
	}{
		{"ABC-123.mp4", "ABC-123", true},
		{"cawd-456_HD.mkv", "CAWD-456", true},
		{"/path/to/SSIS-789.mp4", "SSIS-789", true},
		{"FC2-PPV-1234567.mp4", "FC2-PPV-1234567", true},
		{"random_video.mp4", "", false},
		{"123abc.mp4", "", false},
	}
	for _, c := range cases {
		got, ok := id.Extract(c.input)
		if ok != c.ok {
			t.Errorf("Extract(%q) ok=%v, want %v", c.input, ok, c.ok)
		}
		if ok && got != c.expected {
			t.Errorf("Extract(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}
