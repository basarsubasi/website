package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	store "github.com/basarsubasi/website/gen/sqlc/storage"
	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/platform/backup"
)

const (
	lockFileSuffix         = ".backup.lock"
	saveImageOperation     = "image"
	deleteImageOperation   = "delete-image"
	savePostOperation      = "post"
	deletePostOperation    = "delete-post"
	unknownOperationFormat = "unknown backup operation %q"
	backupItemErrorFormat  = "backup queue item %d: %w"
)

func (r *Store) PendingBackups(ctx context.Context) (int, error) {
	count, err := store.New(r.db).CountBackups(ctx)
	return int(count), err
}

// SyncBackups delivers a bounded FIFO batch. An advisory file lock serializes
// workers across server processes without locking SQLite during I/O.
// Crash recovery replays the unacknowledged operation; delivery is at least once.
func (r *Store) SyncBackups(ctx context.Context, backup backup.Target) error {
	return r.syncBackups(ctx, backup, false)
}

// FlushBackups waits for competing workers and drains the entire queue.
// It must not report success after skipping a lock or only delivering a batch.
func (r *Store) FlushBackups(ctx context.Context, backup backup.Target) error {
	return r.syncBackups(ctx, backup, true)
}

func (r *Store) syncBackups(ctx context.Context, backup backup.Target, wait bool) error {
	lock, err := os.OpenFile(r.path+lockFileSuffix, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	for {
		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		if !wait {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	for i := 0; wait || i < 100; i++ {
		item, err := store.New(r.db).NextBackup(ctx)
		sequence, operation, raw := item.Sequence, item.Operation, item.Payload
		if errors.Is(err, sql.ErrNoRows) {
			if wait {
				return nil
			}
			return backup.Maintain(ctx)
		}
		if err != nil {
			return err
		}
		switch operation {
		case saveImageOperation, deleteImageOperation:
			var p photo.Image
			if err = json.Unmarshal(raw, &p); err == nil {
				if operation == saveImageOperation {
					err = backup.SaveImage(ctx, p)
				} else {
					err = backup.RemoveImage(ctx, p)
				}
			}
		case savePostOperation, deletePostOperation:
			var p article.Post
			if err = json.Unmarshal(raw, &p); err == nil {
				if operation == savePostOperation {
					err = backup.SavePost(ctx, p)
				} else {
					err = backup.RemovePost(ctx, p)
				}
			}
		default:
			err = fmt.Errorf(unknownOperationFormat, operation)
		}
		if err != nil {
			return fmt.Errorf(backupItemErrorFormat, sequence, err)
		}
		if err = store.New(r.db).DeleteBackup(ctx, sequence); err != nil {
			return err
		}
	}
	return nil
}
