package usecase

import (
	"context"
	"github.com/basarsubasi/website/internal/domain"
	"regexp"
	"strings"
	"uuid"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (s *Service) ListPosts(ctx context.Context, p domain.Pagination, includeDrafts bool) (domain.Page[domain.Post], error) {
	if err := validPage(p); err != nil {
		return domain.Page[domain.Post]{}, err
	}
	return s.repo.ListPosts(ctx, p, includeDrafts)
}
func (s *Service) GetPost(ctx context.Context, slug string) (domain.Post, error) {
	return s.repo.GetPost(ctx, slug)
}
func (s *Service) SavePost(ctx context.Context, id string, p domain.PostInput) (domain.Post, error) {
	p.Slug = strings.TrimSpace(p.Slug)
	p.Title = strings.TrimSpace(p.Title)
	if !slugPattern.MatchString(p.Slug) || len(p.Slug) > 200 || p.Title == "" || len(p.Title) > 300 || strings.TrimSpace(p.Body) == "" || (p.Published != 0 && p.Published != 1) {
		return domain.Post{}, domain.Invalid("Provide a title, body, lowercase hyphenated slug, and published value 0 or 1")
	}
	if id != "" {
		return s.repo.UpdatePost(ctx, id, p)
	}
	return s.repo.CreatePost(ctx, domain.Post{ID: uuid.NewV4().String(), Slug: p.Slug, Title: p.Title, Excerpt: p.Excerpt, Body: p.Body, Published: p.Published})
}
func (s *Service) DeletePost(ctx context.Context, id string) error { return s.repo.DeletePost(ctx, id) }
