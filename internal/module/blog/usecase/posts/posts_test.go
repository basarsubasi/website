package posts

import (
	"context"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
)

const (
	postSlug                = "post"
	postTitle               = "Title"
	postBody                = "Body"
	excerptCharacter        = "ş"
	newPostID               = ""
	validExcerptFailure     = "valid Turkish excerpt rejected"
	extraCharacter          = "x"
	oversizedExcerptFailure = "oversized excerpt saved"
)

type postWriter struct {
	Repository
	saved bool
}

func (r *postWriter) CreatePost(_ context.Context, p article.Post) (article.Post, error) {
	r.saved = true
	return p, nil
}
func TestExcerptLimitCountsCharactersBeforeSaving(t *testing.T) {
	repo := &postWriter{}
	service := New(repo)
	input := article.PostInput{Slug: postSlug, Title: postTitle, Body: postBody, Excerpt: strings.Repeat(excerptCharacter, MaxExcerptLength)}
	if _, err := service.SavePost(context.Background(), newPostID, input); err != nil || !repo.saved {
		t.Fatal(validExcerptFailure, err)
	}
	repo.saved = false
	input.Excerpt += extraCharacter
	if _, err := service.SavePost(context.Background(), newPostID, input); err == nil || repo.saved {
		t.Fatal(oversizedExcerptFailure)
	}
}
