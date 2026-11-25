package images

import (
	"context"
	"io"
)

type Storage interface {
	Put(context.Context, string, io.Reader, int64, string, map[string]string) (string, error)
	Delete(context.Context, string) error
}

type UseCase struct {
	blogImages Storage
}

func New(storage Storage) *UseCase {
	return &UseCase{blogImages: storage}
}
