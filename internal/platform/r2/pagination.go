package r2

import (
	"github.com/basarsubasi/website/internal/shared/domain/pagination"
)

func paginate[T any](items []T, p pagination.Pagination) pagination.Page[T] {
	total := int64(len(items))
	start := (p.Page - 1) * p.Limit
	if start > total {
		start = total
	}
	end := start + p.Limit
	if end > total {
		end = total
	}
	return pagination.Page[T]{Data: items[start:end], Total: total, Page: p.Page, Limit: p.Limit}
}
