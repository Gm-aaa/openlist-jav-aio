package state

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Record struct {
	ID            int64
	OpenListPath  string
	JavID         string
	ScrapeDone    bool
	StrmDone      bool
	SubtitleDone  bool
	TranslateDone bool
	ErrorMsg      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// EnabledSteps indicates which pipeline steps are active in the current config.
// ListIncomplete only considers a record incomplete for enabled steps.
type EnabledSteps struct {
	Scrape    bool
	STRM      bool
	Subtitle  bool
	Translate bool
}

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	return &DB{sql: db}, nil
}

func (d *DB) Close() error { return d.sql.Close() }

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS processed_files (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    openlist_path  TEXT NOT NULL UNIQUE,
    jav_id         TEXT NOT NULL,
    scrape_done    INTEGER NOT NULL DEFAULT 0,
    strm_done      INTEGER NOT NULL DEFAULT 0,
    subtitle_done  INTEGER NOT NULL DEFAULT 0,
    translate_done INTEGER NOT NULL DEFAULT 0,
    error_msg      TEXT NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_jav_id ON processed_files(jav_id);
`)
	return err
}

func (d *DB) Upsert(r *Record) error {
	_, err := d.sql.Exec(`
INSERT INTO processed_files (openlist_path, jav_id, scrape_done, strm_done, subtitle_done, translate_done, error_msg, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(openlist_path) DO UPDATE SET
    scrape_done=excluded.scrape_done,
    strm_done=excluded.strm_done,
    subtitle_done=excluded.subtitle_done,
    translate_done=excluded.translate_done,
    error_msg=excluded.error_msg,
    updated_at=CURRENT_TIMESTAMP`,
		r.OpenListPath, r.JavID,
		boolInt(r.ScrapeDone), boolInt(r.StrmDone),
		boolInt(r.SubtitleDone), boolInt(r.TranslateDone),
		r.ErrorMsg,
	)
	return err
}

func (d *DB) Get(openlistPath string) (*Record, error) {
	row := d.sql.QueryRow(
		`SELECT id, openlist_path, jav_id, scrape_done, strm_done, subtitle_done, translate_done, error_msg, created_at, updated_at
		 FROM processed_files WHERE openlist_path=?`, openlistPath)
	return scanRecord(row)
}

// ListIncomplete returns records that have at least one enabled step not yet completed.
func (d *DB) ListIncomplete(steps EnabledSteps) ([]*Record, error) {
	var conditions []string
	if steps.Scrape    { conditions = append(conditions, "scrape_done=0") }
	if steps.STRM      { conditions = append(conditions, "strm_done=0") }
	if steps.Subtitle  { conditions = append(conditions, "subtitle_done=0") }
	if steps.Translate { conditions = append(conditions, "translate_done=0") }

	where := "1=0" // no enabled steps → nothing is incomplete
	if len(conditions) > 0 {
		where = strings.Join(conditions, " OR ")
	}

	rows, err := d.sql.Query(
		`SELECT id, openlist_path, jav_id, scrape_done, strm_done, subtitle_done, translate_done, error_msg, created_at, updated_at
		 FROM processed_files WHERE ` + where)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []*Record
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRecord(s scanner) (*Record, error) {
	var r Record
	var scrapeDone, strmDone, subtitleDone, translateDone int
	err := s.Scan(&r.ID, &r.OpenListPath, &r.JavID,
		&scrapeDone, &strmDone, &subtitleDone, &translateDone,
		&r.ErrorMsg, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.ScrapeDone = scrapeDone == 1
	r.StrmDone = strmDone == 1
	r.SubtitleDone = subtitleDone == 1
	r.TranslateDone = translateDone == 1
	return &r, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
