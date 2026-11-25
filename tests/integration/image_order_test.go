package integration_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/platform/r2"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	imageOrderTestDatabaseFilename = "site.db"
	photoIDFormat                  = "photo-%02d"
	photosPrefix                   = "photos/"
	pngSuffix                      = ".png"
	imageOrderTestPhotoContents    = "photo"
	imageOrderTestPngContentType   = "image/png"
	imageOrderTestBlackAndWhite    = "bw"
	allColors                      = ""
	orderMismatchFormat            = "order: %v want %v"
	moveDown                       = "down"
	moveUp                         = "up"
	missingPhotoID                 = "missing"
	invalidDirection               = "sideways"
	invalidDirectionFailure        = "invalid direction accepted"
	metadataChangedFailure         = "move changed photo metadata"
	imageOrderTestRestoredFilename = "restored.db"
	newestPhotoID                  = "newest"
)

func TestImageOrderPersistsAcrossPagesRestartAndBackup(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	backup := r2.NewBackup(objects)
	path := filepath.Join(t.TempDir(), imageOrderTestDatabaseFilename)
	repo := open(t, path, backup)
	expected := []string{}
	for i := 0; i < 22; i++ {
		id := fmt.Sprintf(photoIDFormat, i)
		key := photosPrefix + id + pngSuffix
		if _, err := objects.Put(ctx, key, strings.NewReader(imageOrderTestPhotoContents), 5, imageOrderTestPngContentType, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.gallery.CreateImage(ctx, photo.Image{UUID: id, ObjectKey: key, Color: imageOrderTestBlackAndWhite, FileType: imageOrderTestPngContentType}); err != nil {
			t.Fatal(err)
		}
		expected = append([]string{id}, expected...)
	}
	check := func(r *testStore) {
		t.Helper()
		for _, color := range []string{allColors, imageOrderTestBlackAndWhite} {
			ids := []string{}
			for page := 1; page <= 2; page++ {
				got, err := r.gallery.ListImages(ctx, domain.Pagination{Page: int64(page), Limit: 20}, color)
				if err != nil {
					t.Fatal(err)
				}
				for _, image := range got.Data {
					ids = append(ids, image.UUID)
				}
			}
			if !reflect.DeepEqual(ids, expected) {
				t.Fatalf(orderMismatchFormat, ids, expected)
			}
		}
	}
	check(repo)
	original, err := repo.gallery.GetImage(ctx, expected[19])
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.gallery.MoveImage(ctx, expected[19], moveDown); err != nil {
		t.Fatal(err)
	}
	expected[19], expected[20] = expected[20], expected[19]
	check(repo)
	if err := repo.gallery.MoveImage(ctx, expected[20], moveUp); err != nil {
		t.Fatal(err)
	}
	expected[19], expected[20] = expected[20], expected[19]
	if err := repo.gallery.MoveImage(ctx, expected[0], moveDown); err != nil {
		t.Fatal(err)
	}
	expected[0], expected[1] = expected[1], expected[0]
	// Boundary moves are no-ops; invalid inputs cannot change the order.
	for _, move := range []struct{ id, direction string }{{expected[0], moveUp}, {expected[21], moveDown}} {
		if err := repo.gallery.MoveImage(ctx, move.id, move.direction); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.gallery.MoveImage(ctx, missingPhotoID, moveUp); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	if err := repo.gallery.MoveImage(ctx, expected[0], invalidDirection); err == nil {
		t.Fatal(invalidDirectionFailure)
	}
	check(repo)
	got, err := repo.gallery.GetImage(ctx, original.UUID)
	if err != nil || got.CreatedAt != original.CreatedAt || got.ObjectKey != original.ObjectKey {
		t.Fatal(metadataChangedFailure, got, err)
	}
	repo.Close()
	repo = open(t, path, backup)
	check(repo)
	if err := repo.SyncBackups(ctx, backup); err != nil {
		t.Fatal(err)
	}
	restored := open(t, filepath.Join(t.TempDir(), imageOrderTestRestoredFilename), r2.NewBackup(objects))
	check(restored)
	if _, err := restored.gallery.CreateImage(ctx, photo.Image{UUID: newestPhotoID, Color: imageOrderTestBlackAndWhite}); err != nil {
		t.Fatal(err)
	}
	expected = append([]string{newestPhotoID}, expected...)
	check(restored)
}
