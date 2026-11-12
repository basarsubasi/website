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

	"github.com/basarsubasi/website/internal/adapters/database/dbquery"
	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/usecase"
)

func (r *Repository) PendingBackups(ctx context.Context) (int, error) {
	count, err := dbquery.New(r.db).CountBackups(ctx)
	return int(count), err
}

// SyncBackups delivers a bounded FIFO batch. An advisory file lock serializes
// workers across the server and seed commands without locking SQLite during I/O.
// Crash recovery replays the unacknowledged operation; delivery is at least once.
func (r *Repository) SyncBackups(ctx context.Context, backup usecase.Backup) error {
	return r.syncBackups(ctx, backup, false)
}

// FlushBackups waits for competing workers and drains the entire queue.
// It must not report success after skipping a lock or only delivering a batch.
func (r *Repository) FlushBackups(ctx context.Context, backup usecase.Backup) error {
	return r.syncBackups(ctx, backup, true)
}

func (r *Repository) syncBackups(ctx context.Context, backup usecase.Backup, wait bool) error {
	lock, err := os.OpenFile(r.path+".backup.lock", os.O_CREATE|os.O_RDWR, 0600)
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
		item, err := dbquery.New(r.db).NextBackup(ctx)
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
		case "image", "delete-image":
			var p domain.Image
			if err = json.Unmarshal(raw, &p); err == nil {
				if operation == "image" {
					err = backup.SaveImage(ctx, p)
				} else {
					err = backup.RemoveImage(ctx, p)
				}
			}
		case "post", "delete-post":
			var p domain.Post
			if err = json.Unmarshal(raw, &p); err == nil {
				if operation == "post" {
					err = backup.SavePost(ctx, p)
				} else {
					err = backup.RemovePost(ctx, p)
				}
			}
		default:
			err = fmt.Errorf("unknown backup operation %q", operation)
		}
		if err != nil {
			return fmt.Errorf("backup queue item %d: %w", sequence, err)
		}
		if err = dbquery.New(r.db).DeleteBackup(ctx, sequence); err != nil {
			return err
		}
	}
	return nil
}
