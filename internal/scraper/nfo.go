package scraper

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openlist-jav-aio/jav-aio/internal/util"
)

type MovieInfo struct {
	ID      string
	Title   string
	Studio  string
	Actors  []string
	Tags    []string
	Rating  float64
	Cover   string
	Year    int
	Runtime int // minutes
}

type nfoMovie struct {
	XMLName  xml.Name   `xml:"movie"`
	Title    string     `xml:"title"`
	Studio   string     `xml:"studio"`
	Rating   float64    `xml:"rating"`
	Year     int        `xml:"year,omitempty"`
	Runtime  int        `xml:"runtime,omitempty"`
	UniqueID string     `xml:"uniqueid"`
	Actors   []nfoActor `xml:"actor"`
	Tags     []string   `xml:"tag"`
	Thumb    string     `xml:"thumb,omitempty"`
}

type nfoActor struct {
	Name string `xml:"name"`
}

func WriteNFO(outDir, javID string, info *MovieInfo) error {
	actors := make([]nfoActor, len(info.Actors))
	for i, a := range info.Actors {
		actors[i] = nfoActor{Name: a}
	}
	movie := nfoMovie{
		Title:    info.Title,
		Studio:   info.Studio,
		Rating:   info.Rating,
		Year:     info.Year,
		Runtime:  info.Runtime,
		UniqueID: info.ID,
		Actors:   actors,
		Tags:     info.Tags,
		Thumb:    info.Cover,
	}
	data, err := xml.MarshalIndent(movie, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal NFO: %w", err)
	}
	content := []byte(xml.Header + strings.TrimSpace(string(data)) + "\n")
	dest := filepath.Join(outDir, javID+".nfo")
	tmpFile := dest + ".tmp"
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		return err
	}
	return util.AtomicRename(tmpFile, dest)
}
