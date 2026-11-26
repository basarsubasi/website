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

	"modernc.org/sqlite"

	"github.com/basarsubasi/website/database/migrations"
	"github.com/basarsubasi/website/internal/platform/backup"
	"github.com/basarsubasi/website/internal/shared/domain/storage"
)

const (
	emptyPath           = ""
	pathRequiredMessage = "DATABASE_PATH is required"
	fileScheme          = "file"
	pragmaParameter     = "_pragma"
	busyTimeoutPragma   = "busy_timeout(5000)"
	journalModePragma   = "journal_mode(WAL)"
	synchronousPragma   = "synchronous(FULL)"
	sqliteDriver        = "sqlite"
	tempDirPrefix       = "website-"
)

// Fixed precision keeps timestamp strings in chronological sort order.
const timestampLayout = "2006-01-02T15:04:05.000000000Z"

type Store struct {
	db           *sql.DB
	path         string
	backup       backup.Target
	writes       chan struct{}
	ephemeralDir string
}

func (r *Store) DB() *sql.DB { return r.db }

func Open(ctx context.Context, path string) (*Store, error) {
	if path == emptyPath {
		return nil, fmt.Errorf(pathRequiredMessage)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: fileScheme, Path: absolute}
	q := url.Values{}
	q.Add(pragmaParameter, busyTimeoutPragma)
	q.Add(pragmaParameter, journalModePragma)
	q.Add(pragmaParameter, synchronousPragma)
	uri.RawQuery = q.Encode()
	db, err := sql.Open(sqliteDriver, uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	repo := &Store{db: db, path: absolute, writes: make(chan struct{}, 1)}
	if err := migrations.Apply(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return repo, nil
}

// OpenEphemeral creates a fresh database for this process without touching an
// existing database or volume. A cold start always restores from R2.
func OpenEphemeral(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(filepath.Dir(path), tempDirPrefix)
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
func (r *Store) RequireBackup(backup backup.Target) { r.backup = backup }

func (r *Store) Close() error {
	err := r.db.Close()
	if r.ephemeralDir != emptyPath {
		err = errors.Join(err, os.RemoveAll(r.ephemeralDir))
	}
	return err
}

func (r *Store) Ping(ctx context.Context) error { return r.db.PingContext(ctx) }

func dbError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return storage.ErrNotFound
	}
	var e *sqlite.Error
	if errors.As(err, &e) && (e.Code() == 1555 || e.Code() == 2067) {
		return storage.ErrConflict
	}
	return err
}

func Now() string { return time.Now().UTC().Format(timestampLayout) }
