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
	locked, err := acquireBackupLock(ctx, lock, wait)
	if err != nil || !locked {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return r.drainBackupQueue(ctx, backup, wait)
}

func (r *Store) drainBackupQueue(ctx context.Context, backup backup.Target, wait bool) error {
	queue := store.New(r.db)
	for i := 0; wait || i < 100; i++ {
		item, err := queue.NextBackup(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			if wait {
				return nil
			}
			return backup.Maintain(ctx)
		}
		if err != nil {
			return err
		}
		if err := deliverBackup(ctx, backup, item); err != nil {
			return fmt.Errorf(backupItemErrorFormat, item.Sequence, err)
		}
		if err := queue.DeleteBackup(ctx, item.Sequence); err != nil {
			return err
		}
	}
	return nil
}

func acquireBackupLock(ctx context.Context, lock *os.File, wait bool) (bool, error) {
	for {
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return false, err
		}
		if !wait {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func deliverBackup(ctx context.Context, target backup.Target, item store.BackupQueue) error {
	switch item.Operation {
	case saveImageOperation:
		return deliver(ctx, item.Payload, target.SaveImage)
	case deleteImageOperation:
		return deliver(ctx, item.Payload, target.RemoveImage)
	case savePostOperation:
		return deliver(ctx, item.Payload, target.SavePost)
	case deletePostOperation:
		return deliver(ctx, item.Payload, target.RemovePost)
	default:
		return fmt.Errorf(unknownOperationFormat, item.Operation)
	}
}

func deliver[T any](ctx context.Context, raw []byte, apply func(context.Context, T) error) error {
	var record T
	if err := json.Unmarshal(raw, &record); err != nil {
		return err
	}
	return apply(ctx, record)
}
