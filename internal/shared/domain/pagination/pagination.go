package pagination

import "github.com/basarsubasi/website/internal/shared/domain/validation"

const invalidPaginationMessage = "Invalid pagination"

type Pagination struct{ Page, Limit int64 }

type Page[T any] struct {
	Data  []T   `json:"data"`
	Total int64 `json:"total"`
	Page  int64 `json:"page"`
	Limit int64 `json:"limit"`
}

func (p Pagination) Validate() error {
	if p.Page < 1 || p.Page > 1000000 || p.Limit < 1 || p.Limit > 100 {
		return validation.Invalid(invalidPaginationMessage)
	}
	return nil
}
