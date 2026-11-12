package usecase

import "context"

type Service struct {
	repo       Repository
	photos     PhotoStorage
	blogImages PhotoStorage
}

func New(repo Repository, photos, blogImages PhotoStorage) *Service {
	return &Service{repo: repo, photos: photos, blogImages: blogImages}
}

func (s *Service) Health(ctx context.Context) error { return s.repo.Ping(ctx) }
