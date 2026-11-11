package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/usecase"
)

func (r *Repository) PendingBackups(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM backup_queue").Scan(&count)
	return count, err
}

// SyncBackups delivers a bounded FIFO batch. An advisory file lock serializes
// workers across the server and seed commands without locking SQLite during I/O.
// Crash recovery replays the unacknowledged operation; delivery is at least once.
func (r *Repository) SyncBackups(ctx context.Context, backup usecase.Backup) error {
	lock, err := os.OpenFile(r.path+".backup.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil
		}
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	for i := 0; i < 100; i++ {
		var sequence int64
		var operation string
		var raw []byte
		err = r.db.QueryRowContext(ctx, "SELECT sequence,operation,payload FROM backup_queue ORDER BY sequence LIMIT 1").Scan(&sequence, &operation, &raw)
		if errors.Is(err, sql.ErrNoRows) {
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
		if _, err = r.db.ExecContext(ctx, "DELETE FROM backup_queue WHERE sequence=?", sequence); err != nil {
			return err
		}
	}
	return nil
}
