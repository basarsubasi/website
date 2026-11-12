package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/basarsubasi/website/internal/adapters/database/dbquery"
	"github.com/basarsubasi/website/internal/domain"
)

func decodePayload[T any](raw []byte, err error) (T, error) {
	var result T
	if err != nil {
		return result, dbError(err)
	}
	err = json.Unmarshal(raw, &result)
	return result, err
}

// Count and page reads share a snapshot so concurrent writes cannot skew totals.
func listPage[T any](ctx context.Context, db *sql.DB, p domain.Pagination, count func(*dbquery.Queries) (int64, error), list func(*dbquery.Queries) ([][]byte, error)) (domain.Page[T], error) {
	result := domain.Page[T]{Data: []T{}, Page: p.Page, Limit: p.Limit}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	q := dbquery.New(tx)
	total, err := count(q)
	if err != nil {
		return result, err
	}
	result.Total = total
	rows, err := list(q)
	if err != nil {
		return result, err
	}
	for _, raw := range rows {
		value, err := decodePayload[T](raw, nil)
		if err != nil {
			return result, err
		}
		result.Data = append(result.Data, value)
	}
	return result, tx.Commit()
}
