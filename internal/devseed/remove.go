package devseed

import (
	"context"
	"errors"
	"fmt"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/usecase"
)

// Remove deletes only known fixture IDs, including edited fixtures.
// The repository removes records and durably queues object cleanup.
func Remove(ctx context.Context, repo usecase.Repository) (int, error) {
	if repo == nil {
		return 0, domain.ErrUnavailable
	}
	count := 0
	for _, id := range photoIDs {
		if err := repo.DeleteImage(ctx, id); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				continue
			}
			return count, fmt.Errorf("remove seed photo %s: %w", id, err)
		}
		count++
	}
	for _, post := range samplePosts() {
		if err := repo.DeletePost(ctx, post.ID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				continue
			}
			return count, fmt.Errorf("remove seed post %s: %w", post.ID, err)
		}
		count++
	}
	return count, nil
}
