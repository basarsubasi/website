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
	db           *sql.DB
	path         string
	backup       usecase.Backup
	writes       chan struct{}
	ephemeralDir string
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
	repo := &Repository{db: db, path: absolute, writes: make(chan struct{}, 1)}
	_, err = db.ExecContext(ctx, schemaSQL)
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

// OpenEphemeral creates a fresh database for this process without touching an
// existing database or volume. A cold start always restores from R2.
func OpenEphemeral(ctx context.Context, path string) (*Repository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(filepath.Dir(path), "website-")
	if err != nil {
		return nil, err
	}
	repo, err := Open(ctx, filepath.Join(dir, filepath.Base(path)))
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	repo.ephemeralDir = dir
	return repo, nil
}

// RequireBackup must be configured before serving requests.
func (r *Repository) RequireBackup(backup usecase.Backup) { r.backup = backup }

func (r *Repository) Close() error {
	err := r.db.Close()
	if r.ephemeralDir != "" {
		err = errors.Join(err, os.RemoveAll(r.ephemeralDir))
	}
	return err
}

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
