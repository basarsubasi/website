// Package database stores website records and pending R2 backup operations.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/usecase"
	"modernc.org/sqlite"
)

// Fixed precision keeps timestamp strings in chronological sort order.
const timestampLayout = "2006-01-02T15:04:05.000000000Z"

type Repository struct {
	db   *sql.DB
	path string
}

var _ usecase.Repository = (*Repository)(nil)

func Open(ctx context.Context, path string) (*Repository, error) {
	if path == "" {
		return nil, fmt.Errorf("DATABASE_PATH is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: absolute}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(FULL)")
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	repo := &Repository{db: db, path: absolute}
	_, err = db.ExecContext(ctx, `
 CREATE TABLE IF NOT EXISTS content (
 kind TEXT NOT NULL CHECK(kind IN ('image','post')),
 id TEXT NOT NULL PRIMARY KEY,
 slug TEXT NOT NULL DEFAULT '',
 color TEXT NOT NULL DEFAULT '',
 published INTEGER NOT NULL DEFAULT 0,
 created_at TEXT NOT NULL,
 payload BLOB NOT NULL
 );
 CREATE UNIQUE INDEX IF NOT EXISTS content_post_slug ON content(slug) WHERE kind='post';
 CREATE INDEX IF NOT EXISTS content_listing ON content(kind,created_at DESC,id DESC);
 CREATE TABLE IF NOT EXISTS backup_queue (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT,
 operation TEXT NOT NULL,
 payload BLOB NOT NULL
 );
 CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY,value TEXT NOT NULL);
 `)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := repo.migratePostSummaries(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return repo, nil
}

func (r *Repository) Close() error { return r.db.Close() }

func (r *Repository) Ping(ctx context.Context) error { return r.db.PingContext(ctx) }

func dbError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	var e *sqlite.Error
	if errors.As(err, &e) && (e.Code() == 1555 || e.Code() == 2067) {
		return domain.ErrConflict
	}
	return err
}

func now() string { return time.Now().UTC().Format(timestampLayout) }
