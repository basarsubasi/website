package r2

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/basarsubasi/website/internal/domain"
)

const maxConcurrentReads = 8
const maxCachedReads = 128
const maxReadCacheBytes = 32 << 20
const readTimeout = 10 * time.Second

type readResult struct {
	state   catalog
	post    domain.Post
	err     error
	size    int
	expires time.Time
}
type readFlight struct {
	done   chan struct{}
	result *readResult
}
type readCache struct {
	mu      sync.Mutex
	entries map[string]*readResult
	flights map[string]*readFlight
	bytes   int
}

func newReadCache() *readCache {
	return &readCache{entries: make(map[string]*readResult), flights: make(map[string]*readFlight)}
}

// Entries are immutable snapshots. Neither cache locks nor repository locks are
// held during I/O. Work is coalesced per key, bounded globally, and time limited.
func (c *readCache) get(ctx context.Context, objects ObjectStore, key string) (*readResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	old := c.entries[key]
	if old != nil && time.Now().Before(old.expires) {
		c.mu.Unlock()
		return old, old.err
	}
	flight := c.flights[key]
	if flight == nil {
		if len(c.flights) >= maxConcurrentReads {
			c.mu.Unlock()
			return nil, domain.ErrBusy
		}
		flight = &readFlight{done: make(chan struct{})}
		c.flights[key] = flight
		go c.fetch(objects, key, old, flight)
	}
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-flight.done:
		return flight.result, flight.result.err
	}
}

func (c *readCache) fetch(objects ObjectStore, key string, old *readResult, flight *readFlight) {
	// A disconnected caller cannot cancel a read shared by other callers.
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	raw, etag, err := objects.Read(ctx, key)
	result := &readResult{err: err, size: len(raw)}
	if err == nil {
		if key == CatalogKey {
			result.state, result.err = parseCatalog(raw, etag)
		} else {
			result.err = json.Unmarshal(raw, &result.post)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// A local write may have published a newer snapshot while this read was
	// in flight. Never replace it with an older R2 response.
	if current := c.entries[key]; current != nil && current != old {
		result = current
	} else {
		c.putLocked(key, result)
	}
	flight.result = result
	delete(c.flights, key)
	close(flight.done)
}

func (c *readCache) publish(key string, result *readResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putLocked(key, result)
}
func (c *readCache) putLocked(key string, result *readResult) {
	if old := c.entries[key]; old != nil {
		c.bytes -= old.size
		delete(c.entries, key)
	}
	// Catalog is pinned: unknown slugs cannot evict it and amplify R2 reads.
	for len(c.entries) >= maxCachedReads || c.bytes+result.size > maxReadCacheBytes {
		var victim string
		var earliest time.Time
		for candidate, entry := range c.entries {
			if candidate != CatalogKey && (victim == "" || entry.expires.Before(earliest)) {
				victim, earliest = candidate, entry.expires
			}
		}
		if victim == "" {
			break
		}
		c.bytes -= c.entries[victim].size
		delete(c.entries, victim)
	}
	result.expires = time.Now().Add(refreshInterval)
	c.entries[key] = result
	c.bytes += result.size
}

func parseCatalog(raw []byte, etag string) (catalog, error) {
	var state catalog
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("invalid catalog JSON: %w", err)
	}
	if state.Version != catalogVersion || state.Revision == "" || state.Images == nil || state.Posts == nil || etag == "" {
		return state, fmt.Errorf("invalid catalog version or structure")
	}
	slugs := map[string]bool{}
	for id, photo := range state.Images {
		if id != photo.Uuid || photo.ObjectKey == "" || (photo.Color != "bw" && photo.Color != "color") {
			return state, fmt.Errorf("invalid image entry in catalog")
		}
	}
	for id, post := range state.Posts {
		if id != post.Metadata.ID || post.ObjectKey == "" || post.Metadata.Slug == "" || slugs[post.Metadata.Slug] {
			return state, fmt.Errorf("invalid post entry in catalog")
		}
		slugs[post.Metadata.Slug] = true
	}
	return state, nil
}

func (r *Repository) snapshot(ctx context.Context) (catalog, error) {
	result, err := r.reads.get(ctx, r.objects, CatalogKey)
	if err != nil {
		return catalog{}, err
	}
	return result.state, nil
}
