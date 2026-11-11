package r2

import (
	"github.com/basarsubasi/website/internal/domain"
)

func paginate[T any](items []T, p domain.Pagination) domain.Page[T] {
	total := int64(len(items))
	start := (p.Page - 1) * p.Limit
	if start > total {
		start = total
	}
	end := start + p.Limit
	if end > total {
		end = total
	}
	return domain.Page[T]{Data: items[start:end], Total: total, Page: p.Page, Limit: p.Limit}
}
