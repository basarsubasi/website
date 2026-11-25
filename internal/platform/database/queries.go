package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/basarsubasi/website/internal/shared/domain"
)

func DecodePayload[T any](raw []byte, err error) (T, error) {
	var result T
	if err != nil {
		return result, dbError(err)
	}
	err = json.Unmarshal(raw, &result)
	return result, err
}

// Count and page reads share a snapshot so concurrent writes cannot skew totals.
func ListPage[T any](ctx context.Context, db *sql.DB, p domain.Pagination, count func(*sql.Tx) (int64, error), list func(*sql.Tx) ([][]byte, error)) (domain.Page[T], error) {
	result := domain.Page[T]{Data: []T{}, Page: p.Page, Limit: p.Limit}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	total, err := count(tx)
	if err != nil {
		return result, err
	}
	result.Total = total
	rows, err := list(tx)
	if err != nil {
		return result, err
	}
	for _, raw := range rows {
		value, err := DecodePayload[T](raw, nil)
		if err != nil {
			return result, err
		}
		result.Data = append(result.Data, value)
	}
	return result, tx.Commit()
}
