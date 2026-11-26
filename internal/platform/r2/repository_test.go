package r2_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"uuid"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/platform/r2"
	"github.com/basarsubasi/website/internal/shared/domain/pagination"
	"github.com/basarsubasi/website/internal/shared/domain/storage"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	firstSlug                  = "first"
	cityName                   = "İstanbul"
	postBody                   = "Body"
	exposedDraftFailure        = "draft exposed through public API"
	missingRestartPost         = "post lost after restart"
	renamedSlug                = "renamed"
	newTitle                   = "New"
	newBody                    = "New body"
	updatedPostLostFailure     = "updated post lost"
	oldSlugFailure             = "old slug resolved"
	duplicateSlugFailure       = "duplicate slug accepted"
	repositoryTestPostsPrefix  = "posts/"
	staleObjectFailure         = "post object not cleaned up"
	restoredDeletedPostFailure = "deleted post returned after restart"

	photosPrefix             = "photos/"
	pngSuffix                = ".png"
	photoContents            = "photo"
	pngContentType           = "image/png"
	blackAndWhite            = "bw"
	publicBaseURL            = "https://example.com/"
	deleteOperation          = "delete"
	r2UnavailableMessage     = "R2 unavailable"
	objectLostFailure        = "failed delete did not preserve object"
	unpersistedDeleteFailure = "catalog delete not persisted"
	pendingDeleteFailure     = "restart did not retry pending deletion"

	committedSlug               = "committed"
	lostAcknowledgedDataFailure = "acknowledged data lost"
	writeOperation              = "write"
	connectionLostMessage       = "connection lost"
	missingAmbiguousErrorFormat = "ambiguous write not signaled: %v"
	changedPostFailure          = "failed write changed the committed post"

	firstChoice        = "one"
	secondChoice       = "two"
	resultCountsFormat = "results %d/%d"
	mergedWriteFailure = "catalog lost or merged an uncommitted write"

	invalidContents            = "invalid"
	jsonContentType            = "application/json"
	silentCatalogResetFailure  = "bad catalog silently reset"
	readOperation              = "read"
	ignoredStartupErrorFailure = "startup ignored R2 failure"

	bucketName                 = "content"
	testCredential             = "test"
	publicContentURL           = "https://content.example.com"
	sdkSlug                    = "sdk"
	sdkTitle                   = "SDK"
	firstTitle                 = "First"
	emptyJSONObject            = "{}"
	acceptedStaleETagFormat    = "stale ETag was not rejected: %v"
	emptyValue                 = ""
	unprotectedCatalogFormat   = "existing catalog was not protected: %v"
	repositoryTestUpdatedTitle = "Updated"
	secondTitle                = "Second"
	sdkRoundTripFormat         = "SDK round trip: got %+v, error %v"
	survivingPostFormat        = "deleted post survived restart: %v"
)

var (
	errR2Unavailable  = errors.New(r2UnavailableMessage)
	errConnectionLost = errors.New(connectionLostMessage)
)

func open(t *testing.T, objects r2.ObjectStore) *r2.Repository {
	t.Helper()
	repo, err := r2.OpenRepository(context.Background(), objects)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}
func TestCatalogSurvivesRestartAndPostEdits(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	repo := open(t, objects)
	id := uuid.NewV4().String()
	created, err := repo.CreatePost(ctx, article.Post{ID: id, Slug: firstSlug, Title: cityName, Body: postBody, Published: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetPost(ctx, firstSlug); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(exposedDraftFailure)
	}
	restarted := open(t, objects)
	draft, err := restarted.GetPostByID(ctx, id)
	if err != nil || draft != created {
		t.Fatal(missingRestartPost)
	}
	updated, err := restarted.UpdatePost(ctx, id, article.PostInput{Slug: renamedSlug, Title: newTitle, Body: newBody, Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	again := open(t, objects)
	post, err := again.GetPost(ctx, renamedSlug)
	if err != nil || post != updated {
		t.Fatal(updatedPostLostFailure)
	}
	if _, err := again.GetPost(ctx, firstSlug); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(oldSlugFailure)
	}
	if _, err := again.CreatePost(ctx, article.Post{ID: uuid.NewV4().String(), Slug: renamedSlug}); !errors.Is(err, storage.ErrConflict) {
		t.Fatal(duplicateSlugFailure)
	}
	if err := again.DeletePost(ctx, id); err != nil {
		t.Fatal(err)
	}
	for _, key := range objects.Keys() {
		if strings.HasPrefix(key, repositoryTestPostsPrefix) {
			t.Fatal(staleObjectFailure)
		}
	}
	if _, err := open(t, objects).GetPostByID(ctx, id); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(restoredDeletedPostFailure)
	}
}
func TestPendingCleanupRetriesAfterRestart(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	repo := open(t, objects)
	key := photosPrefix + uuid.NewV4().String() + pngSuffix
	objects.Put(ctx, key, strings.NewReader(photoContents), 5, pngContentType, nil)
	image := photo.Image{UUID: uuid.NewV4().String(), ObjectKey: key, Color: blackAndWhite, ImageURL: publicBaseURL + key}
	if _, err := repo.CreateImage(ctx, image); err != nil {
		t.Fatal(err)
	}
	objects.FailNext(deleteOperation, errR2Unavailable)
	if err := repo.DeleteImage(ctx, image.UUID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := objects.Read(ctx, key); err != nil {
		t.Fatal(objectLostFailure)
	}
	restarted := open(t, objects)
	if _, err := restarted.GetImage(ctx, image.UUID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(unpersistedDeleteFailure)
	}
	if _, _, err := objects.Read(ctx, key); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(pendingDeleteFailure)
	}
}
func TestAmbiguousCommitIsReconciled(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	repo := open(t, objects)
	objects.AmbiguousNextCommit()
	post, err := repo.CreatePost(ctx, article.Post{ID: uuid.NewV4().String(), Slug: committedSlug, Body: postBody, Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := open(t, objects).GetPost(ctx, committedSlug); err != nil || got.ID != post.ID {
		t.Fatal(lostAcknowledgedDataFailure)
	}
	objects.FailNext(writeOperation, errConnectionLost)
	_, err = repo.UpdatePost(ctx, post.ID, article.PostInput{Slug: committedSlug, Body: newTitle, Published: 1})
	if !errors.Is(err, storage.ErrUncertain) {
		t.Fatalf(missingAmbiguousErrorFormat, err)
	}
	if got, err := open(t, objects).GetPost(ctx, committedSlug); err != nil || got.Body != postBody {
		t.Fatal(changedPostFailure)
	}
}

type racingStore struct {
	*testutil.Objects
	count   atomic.Int32
	barrier chan struct{}
}

func (s *racingStore) PutIf(ctx context.Context, key string, raw []byte, etag string, create bool) (string, error) {
	if !create {
		n := s.count.Add(1)
		if n <= 2 {
			if n == 2 {
				close(s.barrier)
			}
			<-s.barrier
		}
	}
	return s.Objects.PutIf(ctx, key, raw, etag, create)
}
func TestConcurrentCatalogUpdatesCannotOverwrite(t *testing.T) {
	objects := testutil.NewObjects()
	open(t, objects)
	shared := &racingStore{Objects: objects, barrier: make(chan struct{})}
	a, b := open(t, shared), open(t, shared)
	results := make(chan error, 2)
	for i, repo := range []*r2.Repository{a, b} {
		go func(repo *r2.Repository, slug string) {
			_, err := repo.CreatePost(context.Background(), article.Post{ID: uuid.NewV4().String(), Slug: slug, Body: postBody, Published: 1})
			results <- err
		}(repo, []string{firstChoice, secondChoice}[i])
	}
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, storage.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf(resultCountsFormat, successes, conflicts)
	}
	page, err := open(t, objects).ListPosts(context.Background(), pagination.Pagination{Page: 1, Limit: 10}, false)
	if err != nil || page.Total != 1 {
		t.Fatal(mergedWriteFailure)
	}
}
func TestMalformedCatalogFailsStartup(t *testing.T) {
	objects := testutil.NewObjects()
	objects.Put(context.Background(), r2.CatalogKey, strings.NewReader(invalidContents), 7, jsonContentType, nil)
	if _, err := r2.OpenRepository(context.Background(), objects); err == nil {
		t.Fatal(silentCatalogResetFailure)
	}
	objects = testutil.NewObjects()
	objects.FailNext(readOperation, errR2Unavailable)
	if _, err := r2.OpenRepository(context.Background(), objects); err == nil {
		t.Fatal(ignoredStartupErrorFailure)
	}
}

func TestRepositoryThroughS3SDK(t *testing.T) {
	ctx := context.Background()
	server := testutil.S3Server(testutil.NewObjects())
	defer server.Close()
	objects, err := r2.New(r2.Config{Endpoint: server.URL, Bucket: bucketName, AccessKeyID: testCredential, SecretAccessKey: testCredential, PublicURL: publicContentURL})
	if err != nil {
		t.Fatal(err)
	}
	repo := open(t, objects)
	_, oldETag, err := objects.Read(ctx, r2.CatalogKey)
	if err != nil {
		t.Fatal(err)
	}
	post, err := repo.CreatePost(ctx, article.Post{ID: uuid.NewV4().String(), Slug: sdkSlug, Title: sdkTitle, Body: firstTitle, Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := objects.PutIf(ctx, r2.CatalogKey, []byte(emptyJSONObject), oldETag, false); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf(acceptedStaleETagFormat, err)
	}
	if _, err := objects.PutIf(ctx, r2.CatalogKey, []byte(emptyJSONObject), emptyValue, true); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf(unprotectedCatalogFormat, err)
	}
	updated, err := repo.UpdatePost(ctx, post.ID, article.PostInput{Slug: sdkSlug, Title: repositoryTestUpdatedTitle, Body: secondTitle, Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	restarted := open(t, objects)
	got, err := restarted.GetPost(ctx, sdkSlug)
	if err != nil || got != updated {
		t.Fatalf(sdkRoundTripFormat, got, err)
	}
	if err := restarted.DeletePost(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := open(t, objects).GetPostByID(ctx, post.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf(survivingPostFormat, err)
	}
}
