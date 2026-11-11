package usecase

import "context"

type Service struct {
	repo   Repository
	photos PhotoStorage
}

func New(repo Repository, photos PhotoStorage) *Service { return &Service{repo: repo, photos: photos} }

func (s *Service) Health(ctx context.Context) error { return s.repo.Ping(ctx) }
