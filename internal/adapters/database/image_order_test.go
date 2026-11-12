package database_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/adapters/database"
	"github.com/basarsubasi/website/internal/adapters/r2"
	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

func TestImageOrderPersistsAcrossPagesRestartAndBackup(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	path := filepath.Join(t.TempDir(), "site.db")
	repo := open(t, path, backup)
	expected := []string{}
	for i := 0; i < 22; i++ {
		id := fmt.Sprintf("photo-%02d", i)
		key := "photos/" + id + ".png"
		if _, err := objects.Put(ctx, key, strings.NewReader("photo"), 5, "image/png", nil); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.CreateImage(ctx, domain.Image{UUID: id, ObjectKey: key, Color: "bw", FileType: "image/png"}); err != nil {
			t.Fatal(err)
		}
		expected = append([]string{id}, expected...)
	}
	check := func(r *database.Repository) {
		t.Helper()
		for _, color := range []string{"", "bw"} {
			ids := []string{}
			for page := 1; page <= 2; page++ {
				got, err := r.ListImages(ctx, domain.Pagination{Page: int64(page), Limit: 20}, color)
				if err != nil {
					t.Fatal(err)
				}
				for _, image := range got.Data {
					ids = append(ids, image.UUID)
				}
			}
			if !reflect.DeepEqual(ids, expected) {
				t.Fatalf("order: %v want %v", ids, expected)
			}
		}
	}
	check(repo)
	original, err := repo.GetImage(ctx, expected[19])
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MoveImage(ctx, expected[19], "down"); err != nil {
		t.Fatal(err)
	}
	expected[19], expected[20] = expected[20], expected[19]
	check(repo)
	if err := repo.MoveImage(ctx, expected[20], "up"); err != nil {
		t.Fatal(err)
	}
	expected[19], expected[20] = expected[20], expected[19]
	if err := repo.MoveImage(ctx, expected[0], "down"); err != nil {
		t.Fatal(err)
	}
	expected[0], expected[1] = expected[1], expected[0]
	// Boundary moves are no-ops; invalid inputs cannot change the order.
	for _, move := range []struct{ id, direction string }{{expected[0], "up"}, {expected[21], "down"}} {
		if err := repo.MoveImage(ctx, move.id, move.direction); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.MoveImage(ctx, "missing", "up"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	if err := repo.MoveImage(ctx, expected[0], "sideways"); err == nil {
		t.Fatal("invalid direction accepted")
	}
	check(repo)
	got, err := repo.GetImage(ctx, original.UUID)
	if err != nil || got.CreatedAt != original.CreatedAt || got.ObjectKey != original.ObjectKey {
		t.Fatal("move changed photo metadata", got, err)
	}
	repo.Close()
	repo = open(t, path, backup)
	check(repo)
	if err := repo.SyncBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	restored := open(t, filepath.Join(t.TempDir(), "restored.db"), r2.NewBackup(objects))
	check(restored)
	if _, err := restored.CreateImage(ctx, domain.Image{UUID: "newest", Color: "bw"}); err != nil {
		t.Fatal(err)
	}
	expected = append([]string{"newest"}, expected...)
	check(restored)
}
