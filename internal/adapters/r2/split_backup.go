package r2

import (
	"context"
	"errors"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/usecase"
)

// SplitBackup keeps each bucket's catalog and cleanup scoped to its own content.
type SplitBackup struct {
	photos *Backup
	blog   *Backup
}

var _ usecase.Backup = (*SplitBackup)(nil)

func NewSplitBackup(photos, blog ObjectStore) *SplitBackup {
	return &SplitBackup{photos: NewBackup(photos), blog: NewBackup(blog)}
}
func (b *SplitBackup) LoadSnapshot(ctx context.Context) (domain.Snapshot, error) {
	photos, err := b.photos.loadSnapshot(ctx, true, false)
	if err != nil {
		return domain.Snapshot{}, err
	}
	blog, err := b.blog.loadSnapshot(ctx, false, true)
	if err != nil {
		return domain.Snapshot{}, err
	}
	return domain.Snapshot{Images: photos.Images, Posts: blog.Posts}, nil
}
func (b *SplitBackup) SaveImage(ctx context.Context, p domain.Image) error {
	return b.photos.SaveImage(ctx, p)
}
func (b *SplitBackup) RemoveImage(ctx context.Context, p domain.Image) error {
	return b.photos.RemoveImage(ctx, p)
}
func (b *SplitBackup) SavePost(ctx context.Context, p domain.Post) error {
	return b.blog.SavePost(ctx, p)
}
func (b *SplitBackup) RemovePost(ctx context.Context, p domain.Post) error {
	return b.blog.RemovePost(ctx, p)
}
func (b *SplitBackup) Maintain(ctx context.Context) error {
	// Attempt both buckets even if one is unavailable.
	return errors.Join(b.photos.Maintain(ctx), b.blog.Maintain(ctx))
}
