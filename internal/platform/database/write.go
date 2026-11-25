package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	store "github.com/basarsubasi/website/gen/sqlc/storage"
	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	syncOperation        = "synchronize pending changes"
	uncertainWriteFormat = "%w: %v"
)

func InsertImage(ctx context.Context, tx *sql.Tx, image photo.Image) error {
	return insert(ctx, tx, store.InsertContentParams{Kind: imageKind, ID: image.UUID, Color: image.Color, CreatedAt: image.CreatedAt}, image)
}

func InsertPost(ctx context.Context, tx *sql.Tx, post article.Post) error {
	return insert(ctx, tx, store.InsertContentParams{Kind: postKind, ID: post.ID, Slug: post.Slug, Published: post.Published, CreatedAt: post.CreatedAt}, post)
}

func insert(ctx context.Context, tx *sql.Tx, row store.InsertContentParams, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	row.Payload = raw
	return dbError(store.New(tx).InsertContent(ctx, row))
}

func Enqueue(ctx context.Context, tx *sql.Tx, op string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	err = store.New(tx).EnqueueBackup(ctx, store.EnqueueBackupParams{Operation: op, Payload: raw})
	return err
}

func (r *Store) Change(ctx context.Context, fn func(*sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	select {
	case r.writes <- struct{}{}:
		defer func() { <-r.writes }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if r.backup != nil {
		if err := r.FlushBackups(ctx, r.backup); err != nil {
			return &domain.StorageError{Operation: syncOperation, Err: err}
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return dbError(err)
	}
	if err := tx.Commit(); err != nil {
		return dbError(err)
	}
	if r.backup != nil {
		if err := r.FlushBackups(ctx, r.backup); err != nil {
			// The SQLite commit happened: upload cleanup must not delete the object
			// referenced by this record while its backup is pending.
			return fmt.Errorf(uncertainWriteFormat, domain.ErrUncertain, err)
		}
	}
	return nil
}
