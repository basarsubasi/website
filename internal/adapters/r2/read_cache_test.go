package r2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

type observedStore struct {
	*testutil.Objects
	reads        atomic.Int32
	blockPosts   atomic.Bool
	blockCatalog atomic.Bool
	started      chan string
	release      chan struct{}
}

func (s *observedStore) Read(ctx context.Context, key string) ([]byte, string, error) {
	s.reads.Add(1)
	raw, etag, err := s.Objects.Read(ctx, key)
	if (strings.HasPrefix(key, "posts/") && s.blockPosts.Load()) ||
		(key == CatalogKey && s.blockCatalog.CompareAndSwap(true, false)) {
		s.started <- key
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}
	return raw, etag, err
}
func cacheFixture(t *testing.T) (*Repository, *observedStore, func()) {
	t.Helper()
	store := &observedStore{Objects: testutil.NewObjects(), started: make(chan string, 32), release: make(chan struct{})}
	var once sync.Once
	release := func() { once.Do(func() { close(store.release) }) }
	t.Cleanup(release)
	repo, err := OpenRepository(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	return repo, store, release
}
func expireCatalog(repo *Repository) {
	repo.reads.mu.Lock()
	repo.reads.entries[CatalogKey].expires = time.Time{}
	repo.reads.mu.Unlock()
}
func awaitRead(t *testing.T, store *observedStore) {
	t.Helper()
	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("read did not start")
	}
}
func addCachedPost(t *testing.T, repo *Repository, slug string) domain.Post {
	t.Helper()
	post, err := repo.CreatePost(context.Background(), domain.Post{ID: uuid.NewV4().String(), Slug: slug, Title: slug, Body: "body", Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	return post
}
func TestPublicReadsReuseCatalogAndPostCache(t *testing.T) {
	repo, store, _ := cacheFixture(t)
	post := addCachedPost(t, repo, "cached")
	before := store.reads.Load()
	for i := 0; i < 30; i++ {
		if err := repo.Ping(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.GetPost(context.Background(), fmt.Sprintf("missing-%d", i)); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal(err)
		}
		if got, err := repo.GetPost(context.Background(), post.Slug); err != nil || got.ID != post.ID {
			t.Fatalf("post: %+v %v", got, err)
		}
	}
	if reads := store.reads.Load() - before; reads != 1 {
		t.Fatalf("90 public requests caused %d reads, want one post read", reads)
	}
}

func TestConcurrentCatalogRefreshAndFailureAreCoalesced(t *testing.T) {
	repo, store, release := cacheFixture(t)
	expireCatalog(repo)
	store.blockCatalog.Store(true)
	before := store.reads.Load()
	results := make(chan error, 32)
	for i := 0; i < 32; i++ {
		go func() { results <- repo.Ping(context.Background()) }()
	}
	awaitRead(t, store)
	release()
	for i := 0; i < 32; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if reads := store.reads.Load() - before; reads != 1 {
		t.Fatalf("parallel refreshes: %d", reads)
	}
	expireCatalog(repo)
	offline := errors.New("offline")
	store.FailNext("read", offline)
	before = store.reads.Load()
	for i := 0; i < 30; i++ {
		if err := repo.Ping(context.Background()); !errors.Is(err, offline) {
			t.Fatalf("failure not retained: %v", err)
		}
	}
	if reads := store.reads.Load() - before; reads != 1 {
		t.Fatalf("outage amplified into %d reads", reads)
	}
}

func TestSlowPostReadsAreBoundedAndDoNotBlockGallery(t *testing.T) {
	repo, store, release := cacheFixture(t)
	for i := 0; i <= maxConcurrentReads; i++ {
		addCachedPost(t, repo, fmt.Sprintf("post-%d", i))
	}
	store.blockPosts.Store(true)
	results := make(chan error, maxConcurrentReads)
	for i := 0; i < maxConcurrentReads; i++ {
		go func(i int) { _, err := repo.GetPost(context.Background(), fmt.Sprintf("post-%d", i)); results <- err }(i)
		awaitRead(t, store)
	}
	if _, err := repo.GetPost(context.Background(), fmt.Sprintf("post-%d", maxConcurrentReads)); !errors.Is(err, domain.ErrBusy) {
		t.Fatalf("unbounded work: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	waiter := make(chan error, 1)
	go func() { _, err := repo.GetPost(ctx, "post-0"); waiter <- err }()
	cancel()
	select {
	case err := <-waiter:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter remained blocked")
	}
	gallery := make(chan error, 1)
	go func() {
		_, err := repo.ListImages(context.Background(), domain.Pagination{Page: 1, Limit: 10}, "")
		gallery <- err
	}()
	select {
	case err := <-gallery:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("network read holds gallery lock")
	}
	release()
	for i := 0; i < maxConcurrentReads; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestInFlightRefreshCannotReplaceCommittedSnapshot(t *testing.T) {
	repo, store, release := cacheFixture(t)
	expireCatalog(repo)
	store.blockCatalog.Store(true)
	result := make(chan error, 1)
	go func() { result <- repo.Ping(context.Background()) }()
	awaitRead(t, store)
	post := addCachedPost(t, repo, "new")
	release()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetPost(context.Background(), post.Slug); err != nil || got.ID != post.ID {
		t.Fatalf("committed snapshot overwritten: %+v %v", got, err)
	}
}

func TestPostEditsAndDeletesInvalidatePublicVisibility(t *testing.T) {
	repo, _, _ := cacheFixture(t)
	post := addCachedPost(t, repo, "visible")
	if _, err := repo.GetPost(context.Background(), post.Slug); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdatePost(context.Background(), post.ID, domain.PostInput{Slug: post.Slug, Body: "draft", Published: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetPost(context.Background(), post.Slug); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cached draft exposed: %v", err)
	}
	if err := repo.DeletePost(context.Background(), post.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetPostByID(context.Background(), post.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted post returned: %v", err)
	}
}

func TestReadCacheCapacityPinsCatalog(t *testing.T) {
	cache := newReadCache()
	cache.publish(CatalogKey, &readResult{size: 16 << 20})
	for i := 0; i < maxCachedReads*2; i++ {
		cache.publish(fmt.Sprint(i), &readResult{size: 1 << 20})
	}
	if cache.bytes > maxReadCacheBytes || len(cache.entries) > maxCachedReads {
		t.Fatal("cache is unbounded")
	}
	if cache.entries[CatalogKey] == nil {
		t.Fatal("catalog evicted by post reads")
	}
}

func TestConcurrentPostReadsShareOneFetch(t *testing.T) {
	repo, store, release := cacheFixture(t)
	addCachedPost(t, repo, "shared")
	store.blockPosts.Store(true)
	before := store.reads.Load()
	results := make(chan error, 32)
	for i := 0; i < 32; i++ {
		go func() { _, err := repo.GetPost(context.Background(), "shared"); results <- err }()
	}
	awaitRead(t, store)
	release()
	for i := 0; i < 32; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if reads := store.reads.Load() - before; reads != 1 {
		t.Fatalf("post fetched %d times", reads)
	}
}
