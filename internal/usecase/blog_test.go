package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/domain"
)

type postWriter struct {
	Repository
	saved bool
}

func (r *postWriter) CreatePost(_ context.Context, p domain.Post) (domain.Post, error) {
	r.saved = true
	return p, nil
}
func TestExcerptLimitCountsCharactersBeforeSaving(t *testing.T) {
	repo := &postWriter{}
	service := New(repo, nil)
	input := domain.PostInput{Slug: "post", Title: "Title", Body: "Body", Excerpt: strings.Repeat("ş", MaxExcerptLength)}
	if _, err := service.SavePost(context.Background(), "", input); err != nil || !repo.saved {
		t.Fatal("valid Turkish excerpt rejected", err)
	}
	repo.saved = false
	input.Excerpt += "x"
	if _, err := service.SavePost(context.Background(), "", input); err == nil || repo.saved {
		t.Fatal("oversized excerpt saved")
	}
}
