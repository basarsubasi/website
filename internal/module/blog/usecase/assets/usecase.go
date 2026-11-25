package assets

import (
	"context"
	"io"
)

type Storage interface {
	Put(context.Context, string, io.Reader, int64, string, map[string]string) (string, error)
	Delete(context.Context, string) error
}

type UseCase struct {
	storage Storage
}

func New(storage Storage) *UseCase {
	return &UseCase{storage: storage}
}
