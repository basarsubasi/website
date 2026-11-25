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

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/shared/domain"
	"github.com/basarsubasi/website/internal/testutil"
)

const (
	readCacheTestPostsPrefix = "posts/"
	readCacheTestEmptyValue  = ""

	readTimeoutFailure = "read did not start"

	readCacheTestPostBody = "body"

	cachedSlug            = "cached"
	missingIDFormat       = "missing-%d"
	postReadFormat        = "post: %+v %v"
	concurrentReadsFormat = "90 public requests caused %d reads, want one post read"

	refreshCountFormat    = "parallel refreshes: %d"
	offlineMessage        = "offline"
	readOperation         = "read"
	cachedFailureFormat   = "failure not retained: %v"
	outageReadCountFormat = "outage amplified into %d reads"

	postIDFormat         = "post-%d"
	unboundedWorkFormat  = "unbounded work: %v"
	firstPostID          = "post-0"
	blockedWaiterFailure = "canceled waiter remained blocked"
	heldLockFailure      = "network read holds gallery lock"

	newSlug                   = "new"
	overwrittenSnapshotFormat = "committed snapshot overwritten: %+v %v"

	visibleSlug               = "visible"
	draftSlug                 = "draft"
	exposedDraftFormat        = "cached draft exposed: %v"
	returnedDeletedPostFormat = "deleted post returned: %v"

	unboundedCacheFailure = "cache is unbounded"
	evictedCatalogFailure = "catalog evicted by post reads"

	sharedSlug       = "shared"
	fetchCountFormat = "post fetched %d times"
)

var errOffline = errors.New(offlineMessage)

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
	if (strings.HasPrefix(key, readCacheTestPostsPrefix) && s.blockPosts.Load()) ||
		(key == CatalogKey && s.blockCatalog.CompareAndSwap(true, false)) {
		s.started <- key
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, readCacheTestEmptyValue, ctx.Err()
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
		t.Fatal(readTimeoutFailure)
	}
}
func addCachedPost(t *testing.T, repo *Repository, slug string) article.Post {
	t.Helper()
	post, err := repo.CreatePost(context.Background(), article.Post{ID: uuid.NewV4().String(), Slug: slug, Title: slug, Body: readCacheTestPostBody, Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	return post
}
func TestPublicReadsReuseCatalogAndPostCache(t *testing.T) {
	repo, store, _ := cacheFixture(t)
	post := addCachedPost(t, repo, cachedSlug)
	before := store.reads.Load()
	for i := 0; i < 30; i++ {
		if err := repo.Ping(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.GetPost(context.Background(), fmt.Sprintf(missingIDFormat, i)); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal(err)
		}
		if got, err := repo.GetPost(context.Background(), post.Slug); err != nil || got.ID != post.ID {
			t.Fatalf(postReadFormat, got, err)
		}
	}
	if reads := store.reads.Load() - before; reads != 1 {
		t.Fatalf(concurrentReadsFormat, reads)
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
		t.Fatalf(refreshCountFormat, reads)
	}
	expireCatalog(repo)
	store.FailNext(readOperation, errOffline)
	before = store.reads.Load()
	for i := 0; i < 30; i++ {
		if err := repo.Ping(context.Background()); !errors.Is(err, errOffline) {
			t.Fatalf(cachedFailureFormat, err)
		}
	}
	if reads := store.reads.Load() - before; reads != 1 {
		t.Fatalf(outageReadCountFormat, reads)
	}
}

func TestSlowPostReadsAreBoundedAndDoNotBlockGallery(t *testing.T) {
	repo, store, release := cacheFixture(t)
	for i := 0; i <= maxConcurrentReads; i++ {
		addCachedPost(t, repo, fmt.Sprintf(postIDFormat, i))
	}
	store.blockPosts.Store(true)
	results := make(chan error, maxConcurrentReads)
	for i := 0; i < maxConcurrentReads; i++ {
		go func(i int) {
			_, err := repo.GetPost(context.Background(), fmt.Sprintf(postIDFormat, i))
			results <- err
		}(i)
		awaitRead(t, store)
	}
	if _, err := repo.GetPost(context.Background(), fmt.Sprintf(postIDFormat, maxConcurrentReads)); !errors.Is(err, domain.ErrBusy) {
		t.Fatalf(unboundedWorkFormat, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	waiter := make(chan error, 1)
	go func() { _, err := repo.GetPost(ctx, firstPostID); waiter <- err }()
	cancel()
	select {
	case err := <-waiter:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal(blockedWaiterFailure)
	}
	gallery := make(chan error, 1)
	go func() {
		_, err := repo.ListImages(context.Background(), domain.Pagination{Page: 1, Limit: 10}, readCacheTestEmptyValue)
		gallery <- err
	}()
	select {
	case err := <-gallery:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal(heldLockFailure)
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
	post := addCachedPost(t, repo, newSlug)
	release()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetPost(context.Background(), post.Slug); err != nil || got.ID != post.ID {
		t.Fatalf(overwrittenSnapshotFormat, got, err)
	}
}

func TestPostEditsAndDeletesInvalidatePublicVisibility(t *testing.T) {
	repo, _, _ := cacheFixture(t)
	post := addCachedPost(t, repo, visibleSlug)
	if _, err := repo.GetPost(context.Background(), post.Slug); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdatePost(context.Background(), post.ID, article.PostInput{Slug: post.Slug, Body: draftSlug, Published: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetPost(context.Background(), post.Slug); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf(exposedDraftFormat, err)
	}
	if err := repo.DeletePost(context.Background(), post.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetPostByID(context.Background(), post.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf(returnedDeletedPostFormat, err)
	}
}

func TestReadCacheCapacityPinsCatalog(t *testing.T) {
	cache := newReadCache()
	cache.publish(CatalogKey, &readResult{size: 16 << 20})
	for i := 0; i < maxCachedReads*2; i++ {
		cache.publish(fmt.Sprint(i), &readResult{size: 1 << 20})
	}
	if cache.bytes > maxReadCacheBytes || len(cache.entries) > maxCachedReads {
		t.Fatal(unboundedCacheFailure)
	}
	if cache.entries[CatalogKey] == nil {
		t.Fatal(evictedCatalogFailure)
	}
}

func TestConcurrentPostReadsShareOneFetch(t *testing.T) {
	repo, store, release := cacheFixture(t)
	addCachedPost(t, repo, sharedSlug)
	store.blockPosts.Store(true)
	before := store.reads.Load()
	results := make(chan error, 32)
	for i := 0; i < 32; i++ {
		go func() { _, err := repo.GetPost(context.Background(), sharedSlug); results <- err }()
	}
	awaitRead(t, store)
	release()
	for i := 0; i < 32; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if reads := store.reads.Load() - before; reads != 1 {
		t.Fatalf(fetchCountFormat, reads)
	}
}
