package openlist_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlist-jav-aio/jav-aio/internal/openlist"
)

func TestListFiles_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/list" {
			json.NewEncoder(w).Encode(map[string]any{
				"code": 200,
				"data": map[string]any{
					"total": 1,
					"content": []map[string]any{
						{"name": "ABC-123.mp4", "is_dir": false, "size": 1024},
					},
				},
			})
		}
	}))
	defer srv.Close()

	c := openlist.NewClient(srv.URL, "token", openlist.RequestDelay{})
	files, err := c.ListFiles(context.Background(), "/jav", []string{".mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Name != "ABC-123.mp4" {
		t.Errorf("unexpected name: %s", files[0].Name)
	}
}

func TestGetFileURL_WithSign(t *testing.T) {
	c := openlist.NewClient("http://localhost", "token", openlist.RequestDelay{})
	// With sign → uses sign param directly, no API call needed.
	url, err := c.GetFileURL(context.Background(), "/jav/ABC-123.mp4", "abc123sign=:0")
	if err != nil {
		t.Fatal(err)
	}
	expected := "http://localhost/d/jav/ABC-123.mp4?sign=abc123sign=:0"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestGetFileURL_FetchesSign(t *testing.T) {
	// No sign provided → GetFileURL calls /api/fs/get to fetch one.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"code": 200,
			"data": map[string]any{
				"sign": "fetched_sign=:0",
			},
		})
	}))
	defer srv.Close()

	c := openlist.NewClient(srv.URL, "token", openlist.RequestDelay{})
	url, err := c.GetFileURL(context.Background(), "/jav/ABC-123.mp4", "")
	if err != nil {
		t.Fatal(err)
	}
	expected := srv.URL + "/d/jav/ABC-123.mp4?sign=fetched_sign=:0"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestListFiles_Pagination(t *testing.T) {
	// Mock returns page 1 with 1 item and total=200 (> per_page=100),
	// then page 2 with 1 item and total=200 to exercise multi-page loop.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Page int `json:"page"` }
		json.NewDecoder(r.Body).Decode(&req)
		var content []map[string]any
		if req.Page == 1 {
			content = []map[string]any{{"name": "AAA-001.mp4", "is_dir": false, "size": 100}}
		} else {
			content = []map[string]any{{"name": "BBB-002.mp4", "is_dir": false, "size": 100}}
		}
		// total=101 forces a second page request (page 1 covers items 1-100, page 2 covers 101)
		json.NewEncoder(w).Encode(map[string]any{
			"code": 200,
			"data": map[string]any{"total": 101, "content": content},
		})
	}))
	defer srv.Close()

	c := openlist.NewClient(srv.URL, "token", openlist.RequestDelay{})
	files, err := c.ListFiles(context.Background(), "/jav", []string{".mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files from 2 pages, got %d", len(files))
	}
}
