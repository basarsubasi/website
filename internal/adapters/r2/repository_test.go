package r2_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"uuid"

	"github.com/basarsubasi/website/internal/adapters/r2"
	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/testutil"
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
	created, err := repo.CreatePost(ctx, domain.Post{ID: id, Slug: "first", Title: "İstanbul", Body: "Body", Published: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetPost(ctx, "first"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("draft exposed through public API")
	}
	restarted := open(t, objects)
	draft, err := restarted.GetPostByID(ctx, id)
	if err != nil || draft != created {
		t.Fatal("post lost after restart")
	}
	updated, err := restarted.UpdatePost(ctx, id, domain.PostInput{Slug: "renamed", Title: "New", Body: "New body", Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	again := open(t, objects)
	post, err := again.GetPost(ctx, "renamed")
	if err != nil || post != updated {
		t.Fatal("updated post lost")
	}
	if _, err := again.GetPost(ctx, "first"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("old slug resolved")
	}
	if _, err := again.CreatePost(ctx, domain.Post{ID: uuid.NewV4().String(), Slug: "renamed"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("duplicate slug accepted")
	}
	if err := again.DeletePost(ctx, id); err != nil {
		t.Fatal(err)
	}
	for _, key := range objects.Keys() {
		if strings.HasPrefix(key, "posts/") {
			t.Fatal("post object not cleaned up")
		}
	}
	if _, err := open(t, objects).GetPostByID(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("deleted post returned after restart")
	}
}
func TestPendingCleanupRetriesAfterRestart(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	repo := open(t, objects)
	key := "photos/" + uuid.NewV4().String() + ".png"
	objects.Put(ctx, key, strings.NewReader("photo"), 5, "image/png", nil)
	image := domain.Image{UUID: uuid.NewV4().String(), ObjectKey: key, Color: "bw", ImageURL: "https://example.com/" + key}
	if _, err := repo.CreateImage(ctx, image); err != nil {
		t.Fatal(err)
	}
	objects.FailNext("delete", errors.New("R2 unavailable"))
	if err := repo.DeleteImage(ctx, image.UUID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := objects.Read(ctx, key); err != nil {
		t.Fatal("failed delete did not preserve object")
	}
	restarted := open(t, objects)
	if _, err := restarted.GetImage(ctx, image.UUID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("catalog delete not persisted")
	}
	if _, _, err := objects.Read(ctx, key); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("restart did not retry pending deletion")
	}
}
func TestAmbiguousCommitIsReconciled(t *testing.T) {
	ctx := context.Background()
	objects := testutil.NewObjects()
	repo := open(t, objects)
	objects.AmbiguousNextCommit()
	post, err := repo.CreatePost(ctx, domain.Post{ID: uuid.NewV4().String(), Slug: "committed", Body: "Body", Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := open(t, objects).GetPost(ctx, "committed"); err != nil || got.ID != post.ID {
		t.Fatal("acknowledged data lost")
	}
	objects.FailNext("write", errors.New("connection lost"))
	_, err = repo.UpdatePost(ctx, post.ID, domain.PostInput{Slug: "committed", Body: "New", Published: 1})
	if !errors.Is(err, domain.ErrUncertain) {
		t.Fatalf("ambiguous write not signaled: %v", err)
	}
	if got, err := open(t, objects).GetPost(ctx, "committed"); err != nil || got.Body != "Body" {
		t.Fatal("failed write changed the committed post")
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
			_, err := repo.CreatePost(context.Background(), domain.Post{ID: uuid.NewV4().String(), Slug: slug, Body: "Body", Published: 1})
			results <- err
		}(repo, []string{"one", "two"}[i])
	}
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, domain.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("results %d/%d", successes, conflicts)
	}
	page, err := open(t, objects).ListPosts(context.Background(), domain.Pagination{Page: 1, Limit: 10}, false)
	if err != nil || page.Total != 1 {
		t.Fatal("catalog lost or merged an uncommitted write")
	}
}
func TestMalformedCatalogFailsStartup(t *testing.T) {
	objects := testutil.NewObjects()
	objects.Put(context.Background(), r2.CatalogKey, strings.NewReader("invalid"), 7, "application/json", nil)
	if _, err := r2.OpenRepository(context.Background(), objects); err == nil {
		t.Fatal("bad catalog silently reset")
	}
	objects = testutil.NewObjects()
	objects.FailNext("read", errors.New("R2 unavailable"))
	if _, err := r2.OpenRepository(context.Background(), objects); err == nil {
		t.Fatal("startup ignored R2 failure")
	}
}

func TestRepositoryThroughS3SDK(t *testing.T) {
	ctx := context.Background()
	server := testutil.S3Server(testutil.NewObjects())
	defer server.Close()
	objects, err := r2.New(r2.Config{Endpoint: server.URL, Bucket: "content", AccessKeyID: "test", SecretAccessKey: "test", PublicURL: "https://content.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	repo := open(t, objects)
	_, oldETag, err := objects.Read(ctx, r2.CatalogKey)
	if err != nil {
		t.Fatal(err)
	}
	post, err := repo.CreatePost(ctx, domain.Post{ID: uuid.NewV4().String(), Slug: "sdk", Title: "SDK", Body: "First", Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := objects.PutIf(ctx, r2.CatalogKey, []byte("{}"), oldETag, false); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale ETag was not rejected: %v", err)
	}
	if _, err := objects.PutIf(ctx, r2.CatalogKey, []byte("{}"), "", true); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("existing catalog was not protected: %v", err)
	}
	updated, err := repo.UpdatePost(ctx, post.ID, domain.PostInput{Slug: "sdk", Title: "Updated", Body: "Second", Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	restarted := open(t, objects)
	got, err := restarted.GetPost(ctx, "sdk")
	if err != nil || got != updated {
		t.Fatalf("SDK round trip: got %+v, error %v", got, err)
	}
	if err := restarted.DeletePost(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := open(t, objects).GetPostByID(ctx, post.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted post survived restart: %v", err)
	}
}
