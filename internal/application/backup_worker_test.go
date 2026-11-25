package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/basarsubasi/website/internal/platform/backup"
	"github.com/basarsubasi/website/internal/platform/database"
)

const (
	backupDeadlineFailure = "backup attempt did not receive its deadline"
	backupWorkerFailure   = "backup worker returned before maintenance exited"
)

type waitingBackup struct {
	backup.Target
	started chan context.Context
	exited  chan struct{}
}

func (b *waitingBackup) Maintain(ctx context.Context) error {
	defer close(b.exited)
	b.started <- ctx
	<-ctx.Done()
	return ctx.Err()
}

func TestStopBackupsCancelsAndJoinsMaintenance(t *testing.T) {
	repo, err := database.Open(t.Context(), filepath.Join(t.TempDir(), databaseFilename))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	target := &waitingBackup{started: make(chan context.Context, 1), exited: make(chan struct{})}
	app := &Application{Repository: repo, Backup: target}
	stop := app.startBackups(t.Context())
	defer stop()

	select {
	case attempt := <-target.started:
		deadline, ok := attempt.Deadline()
		if !ok || time.Until(deadline) > backupAttemptTimeout {
			t.Fatal(backupDeadlineFailure)
		}
	case <-time.After(3 * time.Second):
		t.Fatal(backupDeadlineFailure)
	}
	stop()
	select {
	case <-target.exited:
	default:
		t.Fatal(backupWorkerFailure)
	}
}
