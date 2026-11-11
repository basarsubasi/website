package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/basarsubasi/website/internal/domain"
)

type scanner interface{ Scan(...any) error }

func scan[T any](row scanner) (T, error) {
	var result T
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return result, dbError(err)
	}
	err := json.Unmarshal(raw, &result)
	return result, err
}

func list[T any](ctx context.Context, db *sql.DB, p domain.Pagination, where string, args ...any) (domain.Page[T], error) {
	result := domain.Page[T]{Data: []T{}, Page: p.Page, Limit: p.Limit}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM content WHERE "+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	args = append(args, p.Limit, (p.Page-1)*p.Limit)
	rows, err := tx.QueryContext(ctx, "SELECT payload FROM content WHERE "+where+" ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scan[T](rows)
		if err != nil {
			return result, err
		}
		result.Data = append(result.Data, v)
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	rows.Close()
	return result, tx.Commit()
}
