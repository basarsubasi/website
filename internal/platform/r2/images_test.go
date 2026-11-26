package r2_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/platform/r2"
	"github.com/basarsubasi/website/internal/shared/domain/pagination"
	"github.com/basarsubasi/website/internal/shared/domain/storage"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	colorImage          = "color"
	countryName         = "Türkiye"
	movePhotoUp         = "up"
	missingImageID      = "missing"
	galleryPageMismatch = "list page %d for %q: got %+v, want %v (total %d)"
	discardedPhotoEdit  = "metadata edit changed the identity or ordering of a photo"
)

func galleryPage(t *testing.T, repo *r2.Repository, page int64, color string, total int64, want ...string) {
	t.Helper()
	result, err := repo.ListImages(t.Context(), pagination.Pagination{Page: page, Limit: 1}, color)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, image := range result.Data {
		ids = append(ids, image.UUID)
	}
	if result.Total != total || result.Page != page || !slices.Equal(ids, want) {
		t.Fatalf(galleryPageMismatch, page, color, result, want, total)
	}
}

func TestGalleryCatalogOrderAndFiltersSurviveReopen(t *testing.T) {
	ctx := t.Context()
	objects := testutil.NewObjects()
	repo := open(t, objects)
	for _, image := range []photo.Image{
		{UUID: firstChoice, ObjectKey: photosPrefix + firstChoice, OrderKey: firstChoice, Color: blackAndWhite},
		{UUID: secondChoice, ObjectKey: photosPrefix + secondChoice, OrderKey: secondChoice, Color: colorImage},
	} {
		if _, err := repo.CreateImage(ctx, image); err != nil {
			t.Fatal(err)
		}
	}
	galleryPage(t, repo, 1, emptyValue, 2, secondChoice)
	galleryPage(t, repo, 2, emptyValue, 2, firstChoice)
	galleryPage(t, repo, 3, emptyValue, 2)
	galleryPage(t, repo, 1, blackAndWhite, 1, firstChoice)
	if err := repo.MoveImage(ctx, firstChoice, movePhotoUp); err != nil {
		t.Fatal(err)
	}
	repo = open(t, objects)
	galleryPage(t, repo, 1, emptyValue, 2, firstChoice)
	before, err := repo.GetImage(ctx, firstChoice)
	if err != nil {
		t.Fatal(err)
	}
	input := photo.PhotoInput{Name: firstTitle, Color: colorImage, City: cityName, Country: countryName, YearTaken: 2025}
	updated, err := repo.UpdateImage(ctx, firstChoice, input)
	if err != nil {
		t.Fatal(err)
	}
	want := before
	want.Name, want.Color, want.City, want.Country, want.YearTaken = input.Name, input.Color, input.City, input.Country, input.YearTaken
	if updated != want {
		t.Fatal(discardedPhotoEdit, updated)
	}
	repo = open(t, objects)
	got, err := repo.GetImage(ctx, firstChoice)
	if err != nil || got != want {
		t.Fatal(got, err)
	}
	galleryPage(t, repo, 1, blackAndWhite, 0)
	galleryPage(t, repo, 1, colorImage, 2, firstChoice)
}

func TestGalleryCatalogRejectsConflictingAndMissingImages(t *testing.T) {
	ctx := t.Context()
	repo := open(t, testutil.NewObjects())
	image := photo.Image{UUID: firstChoice, ObjectKey: photosPrefix + firstChoice, Color: blackAndWhite}
	created, err := repo.CreateImage(ctx, image)
	if err != nil {
		t.Fatal(err)
	}
	image.Name = newTitle
	if _, err := repo.CreateImage(ctx, image); !errors.Is(err, storage.ErrConflict) {
		t.Fatal(err)
	}
	if _, err := repo.UpdateImage(ctx, missingImageID, photo.PhotoInput{Color: colorImage}); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(err)
	}
	if err := repo.MoveImage(ctx, missingImageID, movePhotoUp); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(err)
	}
	if err := repo.DeleteImage(ctx, missingImageID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(err)
	}
	got, err := repo.GetImage(ctx, created.UUID)
	if err != nil || got != created {
		t.Fatal(got, err)
	}
}
