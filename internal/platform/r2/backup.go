package r2

import (
	"context"
	"errors"
	"sync"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/module/gallery/domain/photo"
	"github.com/basarsubasi/website/internal/platform/backup"
	"github.com/basarsubasi/website/internal/shared/domain"
)

// Backup lazily opens R2. Existing SQLite databases can start during an outage.
type Backup struct {
	objects ObjectStore
	mu      sync.Mutex
	repo    *Repository
}

var _ backup.Target = (*Backup)(nil)

func NewBackup(objects ObjectStore) *Backup { return &Backup{objects: objects} }
func (b *Backup) repository(ctx context.Context) (*Repository, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.repo == nil {
		repo, err := OpenRepository(ctx, b.objects)
		if err != nil {
			return nil, err
		}
		b.repo = repo
	}
	return b.repo, nil
}
func (b *Backup) LoadSnapshot(ctx context.Context) (backup.Snapshot, error) {
	return b.loadSnapshot(ctx, true, true)
}
func (b *Backup) loadSnapshot(ctx context.Context, images, posts bool) (backup.Snapshot, error) {
	result := backup.Snapshot{Images: []photo.Image{}, Posts: []article.Post{}}
	repo, err := b.repository(ctx)
	if err != nil {
		return result, err
	}
	repo.writeMu.Lock()
	defer repo.writeMu.Unlock()
	if err = repo.load(ctx); err != nil {
		return result, err
	}
	if images {
		for _, photo := range repo.state.Images {
			result.Images = append(result.Images, photo)
		}
	}
	if posts {
		result.Posts, err = repo.restorePosts(ctx)
	}
	if err != nil {
		return backup.Snapshot{}, err
	}
	return result, nil
}
func (b *Backup) SaveImage(ctx context.Context, p photo.Image) error {
	repo, err := b.repository(ctx)
	if err != nil {
		return err
	}
	metadata, err := photo.PhotoMetadata(photo.PhotoInput{Name: p.Name, Country: p.Country, City: p.City, YearTaken: p.YearTaken, Color: p.Color}, p.UUID)
	if err != nil {
		return err
	}
	if err = b.objects.UpdateMetadata(ctx, p.ObjectKey, p.FileType, metadata); err != nil {
		return err
	}
	repo.writeMu.Lock()
	defer repo.writeMu.Unlock()
	return repo.mutate(ctx, func(next *catalog) error {
		if old, ok := next.Images[p.UUID]; ok && old.ObjectKey != p.ObjectKey {
			next.Garbage = append(next.Garbage, old.ObjectKey)
		}
		next.Images[p.UUID] = p
		return nil
	})
}
func (b *Backup) RemoveImage(ctx context.Context, p photo.Image) error {
	repo, err := b.repository(ctx)
	if err != nil {
		return err
	}
	err = repo.DeleteImage(ctx, p.UUID)
	if errors.Is(err, domain.ErrNotFound) {
		return b.objects.Delete(ctx, p.ObjectKey)
	}
	return err
}
func (b *Backup) SavePost(ctx context.Context, p article.Post) error {
	repo, err := b.repository(ctx)
	if err != nil {
		return err
	}
	_, err = repo.savePost(ctx, p.ID, p.Slug, func(_ postEntry, _ bool) (article.Post, error) {
		// Backup replay upserts the original record, including its timestamps.
		return p, nil
	})
	return err
}
func (b *Backup) RemovePost(ctx context.Context, p article.Post) error {
	repo, err := b.repository(ctx)
	if err != nil {
		return err
	}
	err = repo.DeletePost(ctx, p.ID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	return err
}
func (b *Backup) Maintain(ctx context.Context) error {
	repo, err := b.repository(ctx)
	if err != nil {
		return err
	}
	repo.writeMu.Lock()
	defer repo.writeMu.Unlock()
	if err = repo.load(ctx); err != nil {
		return err
	}
	repo.cleanup(ctx)
	return nil
}
