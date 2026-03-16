package scraper

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/metatube-community/metatube-sdk-go/engine"
	"github.com/metatube-community/metatube-sdk-go/engine/providerid"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/openlist-jav-aio/jav-aio/internal/util"
)

// Scraper wraps the metatube engine to fetch JAV metadata.
type Scraper struct {
	engine   *engine.Engine
	log      *slog.Logger
	language string
	sources  []string
}

// New creates a Scraper using the metatube default in-memory engine.
// preferredSources limits which providers to try (empty = all).
func New(preferredSources []string, language string, log *slog.Logger) (*Scraper, error) {
	e := engine.Default()
	return &Scraper{
		engine:   e,
		log:      log,
		language: language,
		sources:  preferredSources,
	}, nil
}

// Scrape fetches metadata for javID and writes NFO + optional cover to outDir.
func (s *Scraper) Scrape(ctx context.Context, javID, outDir string, downloadCover bool) (*MovieInfo, error) {
	start := time.Now()
	s.log.Debug("scraping", "id", javID)

	// Try each preferred provider first; fall back to SearchMovieAll.
	var detail *model.MovieInfo

	if len(s.sources) > 0 {
		for _, providerName := range s.sources {
			results, err := s.engine.SearchMovie(javID, providerName, true)
			if err != nil || len(results) == 0 {
				continue
			}
			pid, err := providerid.New(results[0].Provider, results[0].ID)
			if err != nil {
				continue
			}
			detail, err = s.engine.GetMovieInfoByProviderID(pid, false)
			if err == nil && detail != nil {
				break
			}
		}
	}

	// Fallback: search all providers.
	if detail == nil {
		results, err := s.engine.SearchMovieAll(javID, true)
		if err != nil || len(results) == 0 {
			return nil, fmt.Errorf("search %s: no results found", javID)
		}
		pid, err := providerid.New(results[0].Provider, results[0].ID)
		if err != nil {
			return nil, fmt.Errorf("build provider ID for %s: %w", javID, err)
		}
		detail, err = s.engine.GetMovieInfoByProviderID(pid, false)
		if err != nil {
			return nil, fmt.Errorf("get detail %s: %w", javID, err)
		}
	}

	info := convertMovieInfo(detail, javID)

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, err
	}
	if err := WriteNFO(outDir, javID, info); err != nil {
		return nil, fmt.Errorf("write NFO: %w", err)
	}

	if downloadCover && info.Cover != "" {
		if err := downloadFile(ctx, info.Cover, filepath.Join(outDir, javID+".jpg")); err != nil {
			s.log.Warn("failed to download cover", "id", javID, "error", err)
		}
	}

	s.log.Debug("scrape done", "id", javID, "duration_ms", time.Since(start).Milliseconds())
	return info, nil
}

// convertMovieInfo maps model.MovieInfo fields to our local MovieInfo struct.
func convertMovieInfo(m *model.MovieInfo, fallbackID string) *MovieInfo {
	id := m.Number
	if id == "" {
		id = m.ID
	}
	if id == "" {
		id = fallbackID
	}

	// Prefer BigCoverURL, then CoverURL.
	cover := m.BigCoverURL
	if cover == "" {
		cover = m.CoverURL
	}

	// Derive year from ReleaseDate (datatypes.Date is an alias for time.Time).
	year := 0
	releaseTime := time.Time(m.ReleaseDate)
	if !releaseTime.IsZero() {
		year = releaseTime.Year()
	}

	actors := make([]string, len(m.Actors))
	copy(actors, m.Actors)

	tags := make([]string, len(m.Genres))
	copy(tags, m.Genres)

	return &MovieInfo{
		ID:      id,
		Title:   m.Title,
		Studio:  m.Maker,
		Actors:  actors,
		Tags:    tags,
		Rating:  m.Score,
		Cover:   cover,
		Year:    year,
		Runtime: m.Runtime,
	}
}

func downloadFile(ctx context.Context, rawURL, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	// Set Referer to the image host's origin so sites that check hotlink
	// protection (e.g. JavBus) serve the image instead of returning 403.
	if u, parseErr := url.Parse(rawURL); parseErr == nil {
		req.Header.Set("Referer", u.Scheme+"://"+u.Host+"/")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", rawURL, resp.StatusCode)
	}
	// Write to temp file then rename for crash safety.
	tmpFile := dest + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	if _, err = io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpFile)
		return err
	}
	return util.AtomicRename(tmpFile, dest)
}

