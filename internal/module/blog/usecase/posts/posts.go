package posts

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"
	"uuid"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	slugExpression        = `^[a-z0-9]+(?:-[a-z0-9]+)*$`
	excerptTooLongMessage = "Excerpt must be at most 2000 characters"
	emptyString           = ""
	invalidPostMessage    = "Provide a title, body, lowercase hyphenated slug, and published value 0 or 1"
)

var slugPattern = regexp.MustCompile(slugExpression)

const MaxExcerptLength = 2000

func (u *UseCase) ListPosts(ctx context.Context, p domain.Pagination, includeDrafts bool) (domain.Page[article.Post], error) {
	if err := p.Validate(); err != nil {
		return domain.Page[article.Post]{}, err
	}
	return u.repo.ListPosts(ctx, p, includeDrafts)
}

func (u *UseCase) GetPost(ctx context.Context, slug string) (article.Post, error) {
	return u.repo.GetPost(ctx, slug)
}

func (u *UseCase) SavePost(ctx context.Context, id string, p article.PostInput) (article.Post, error) {
	if utf8.RuneCountInString(p.Excerpt) > MaxExcerptLength {
		return article.Post{}, domain.Invalid(excerptTooLongMessage)
	}
	p.Slug = strings.TrimSpace(p.Slug)
	p.Title = strings.TrimSpace(p.Title)
	if !slugPattern.MatchString(p.Slug) || len(p.Slug) > 200 || p.Title == emptyString || len(p.Title) > 300 || strings.TrimSpace(p.Body) == emptyString || (p.Published != 0 && p.Published != 1) {
		return article.Post{}, domain.Invalid(invalidPostMessage)
	}
	if id != emptyString {
		return u.repo.UpdatePost(ctx, id, p)
	}
	return u.repo.CreatePost(ctx, article.Post{ID: uuid.NewV4().String(), Slug: p.Slug, Title: p.Title, Excerpt: p.Excerpt, Body: p.Body, Published: p.Published})
}

func (u *UseCase) DeletePost(ctx context.Context, id string) error { return u.repo.DeletePost(ctx, id) }
