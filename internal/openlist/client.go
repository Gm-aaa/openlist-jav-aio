package openlist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"path"
	"strings"
	"time"
)

type RequestDelay struct {
	Min time.Duration
	Max time.Duration
}

type FileInfo struct {
	Name     string
	Path     string
	Size     int64
	Modified time.Time
	Sign     string // OpenList sign token for direct download URL
}

type Client struct {
	baseURL string
	token   string
	delay   RequestDelay
	http    *http.Client
	log     *slog.Logger
}

func NewClient(baseURL, token string, delay RequestDelay) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		delay:   delay,
		http:    &http.Client{Timeout: 30 * time.Second},
		log:     slog.Default(),
	}
}

func (c *Client) WithLogger(l *slog.Logger) *Client {
	c.log = l
	return c
}

func (c *Client) sleep() {
	if c.delay.Max == 0 {
		return
	}
	d := c.delay.Min
	if span := c.delay.Max - c.delay.Min; span > 0 {
		d += time.Duration(rand.Int63n(int64(span)))
	}
	time.Sleep(d)
}

func (c *Client) post(ctx context.Context, endpoint string, body any, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")

	c.log.Debug("openlist request", "endpoint", endpoint, "body", string(b))
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if envelope.Code != 200 {
		return fmt.Errorf("openlist error %d: %s", envelope.Code, envelope.Message)
	}
	if out != nil {
		return json.Unmarshal(envelope.Data, out)
	}
	return nil
}

// ListFiles returns all files under dirPath matching any of the given extensions.
// Handles pagination automatically.
func (c *Client) ListFiles(ctx context.Context, dirPath string, extensions []string) ([]FileInfo, error) {
	extSet := make(map[string]bool, len(extensions))
	for _, e := range extensions {
		extSet[strings.ToLower(e)] = true
	}

	var results []FileInfo
	page, perPage := 1, 100
	for {
		c.sleep()
		var data struct {
			Total   int `json:"total"`
			Content []struct {
				Name     string `json:"name"`
				IsDir    bool   `json:"is_dir"`
				Size     int64  `json:"size"`
				Modified string `json:"modified"`
				Sign     string `json:"sign"`
			} `json:"content"`
		}
		err := c.post(ctx, "/api/fs/list", map[string]any{
			"path": dirPath, "page": page, "per_page": perPage, "refresh": false,
		}, &data)
		if err != nil {
			return nil, fmt.Errorf("list %s page %d: %w", dirPath, page, err)
		}

		for _, item := range data.Content {
			if item.IsDir {
				sub, err := c.ListFiles(ctx, path.Join(dirPath, item.Name), extensions)
				if err != nil {
					c.log.Warn("failed to list subdir", "path", path.Join(dirPath, item.Name), "error", err)
					continue
				}
				results = append(results, sub...)
				continue
			}
			ext := strings.ToLower(path.Ext(item.Name))
			if !extSet[ext] {
				continue
			}
			mod, _ := time.Parse(time.RFC3339, item.Modified)
			results = append(results, FileInfo{
				Name:     item.Name,
				Path:     path.Join(dirPath, item.Name),
				Size:     item.Size,
				Modified: mod,
				Sign:     item.Sign,
			})
		}

		if len(data.Content) == 0 || page*perPage >= data.Total {
			break
		}
		page++
	}
	c.log.Debug("listed files", "path", dirPath, "count", len(results))
	return results, nil
}

// GetSign calls /api/fs/get to retrieve the download sign for a file.
func (c *Client) GetSign(ctx context.Context, filePath string) (string, error) {
	var data struct {
		Sign string `json:"sign"`
	}
	if err := c.post(ctx, "/api/fs/get", map[string]string{"path": filePath}, &data); err != nil {
		return "", fmt.Errorf("get sign for %s: %w", filePath, err)
	}
	return data.Sign, nil
}

// GetFileURL returns the direct download URL for a file.
// sign should be the value from FileInfo.Sign returned by ListFiles.
// If sign is empty, calls /api/fs/get to fetch a fresh sign.
func (c *Client) GetFileURL(ctx context.Context, filePath, sign string) (string, error) {
	c.log.Debug("constructing file URL", "path", filePath)

	if sign == "" {
		var err error
		sign, err = c.GetSign(ctx, filePath)
		if err != nil {
			return "", err
		}
	}

	encodedPath := encodePath(filePath)
	fileURL := c.baseURL + "/d" + encodedPath + "?sign=" + sign
	c.log.Debug("file URL ready", "path", filePath, "url", fileURL)
	return fileURL, nil
}

// encodePath percent-encodes only spaces and non-ASCII characters (e.g. CJK)
// in each segment of a slash-separated path. Characters like @, -, ., ~ that
// are safe in AList paths are preserved as-is, because AList does not decode
// standard percent-encoding for path matching.
func encodePath(p string) string {
	var sb strings.Builder
	sb.Grow(len(p) * 2)
	for _, b := range []byte(p) {
		if b > 127 || b == ' ' {
			fmt.Fprintf(&sb, "%%%02X", b)
		} else {
			sb.WriteByte(b)
		}
	}
	return sb.String()
}
