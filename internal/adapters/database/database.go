// Package database stores website records and pending R2 backup operations.
package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/basarsubasi/website/internal/domain"
	"github.com/basarsubasi/website/internal/usecase"
	"modernc.org/sqlite"
)

type Repository struct {
	db   *sql.DB
	path string
}

var _ usecase.Repository = (*Repository)(nil)

func Open(ctx context.Context, path string) (*Repository, error) {
	if path == "" {
		return nil, fmt.Errorf("DATABASE_PATH is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: absolute}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(FULL)")
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	repo := &Repository{db: db, path: absolute}
	_, err = db.ExecContext(ctx, `
 CREATE TABLE IF NOT EXISTS content (
 kind TEXT NOT NULL CHECK(kind IN ('image','post')),
 id TEXT NOT NULL PRIMARY KEY,
 slug TEXT NOT NULL DEFAULT '',
 color TEXT NOT NULL DEFAULT '',
 published INTEGER NOT NULL DEFAULT 0,
 created_at TEXT NOT NULL,
 payload BLOB NOT NULL
 );
 CREATE UNIQUE INDEX IF NOT EXISTS content_post_slug ON content(slug) WHERE kind='post';
 CREATE INDEX IF NOT EXISTS content_listing ON content(kind,created_at DESC,id DESC);
 CREATE TABLE IF NOT EXISTS backup_queue (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT,
 operation TEXT NOT NULL,
 payload BLOB NOT NULL
 );
 CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY,value TEXT NOT NULL);
 `)
	if err != nil {
		db.Close()
		return nil, err
	}
	return repo, nil
}
func (r *Repository) Close() error                   { return r.db.Close() }
func (r *Repository) Ping(ctx context.Context) error { return r.db.PingContext(ctx) }
func dbError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	var e *sqlite.Error
	if errors.As(err, &e) && (e.Code() == 1555 || e.Code() == 2067) {
		return domain.ErrConflict
	}
	return err
}
func now() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z") }

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
func (r *Repository) GetImage(ctx context.Context, id string) (domain.Image, error) {
	return scan[domain.Image](r.db.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='image' AND id=?", id))
}
func (r *Repository) GetPostByID(ctx context.Context, id string) (domain.Post, error) {
	return scan[domain.Post](r.db.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='post' AND id=?", id))
}
func (r *Repository) GetPost(ctx context.Context, slug string) (domain.Post, error) {
	return scan[domain.Post](r.db.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='post' AND slug=? AND published=1", slug))
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
func (r *Repository) ListImages(ctx context.Context, p domain.Pagination, color string) (domain.Page[domain.Image], error) {
	where := "kind='image'"
	args := []any{}
	if color != "" {
		where += " AND color=?"
		args = append(args, color)
	}
	return list[domain.Image](ctx, r.db, p, where, args...)
}
func (r *Repository) ListPosts(ctx context.Context, p domain.Pagination, drafts bool) (domain.Page[domain.Post], error) {
	where := "kind='post'"
	if !drafts {
		where += " AND published=1"
	}
	page, err := list[domain.Post](ctx, r.db, p, where)
	if !drafts {
		for i := range page.Data {
			page.Data[i].Body = ""
		}
	}
	return page, err
}
func insert(ctx context.Context, tx *sql.Tx, kind, id, slug, color string, published int64, created string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO content(kind,id,slug,color,published,created_at,payload) VALUES(?,?,?,?,?,?,?)", kind, id, slug, color, published, created, raw)
	return dbError(err)
}
func enqueue(ctx context.Context, tx *sql.Tx, op string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO backup_queue(operation,payload) VALUES(?,?)", op, raw)
	return err
}
func (r *Repository) change(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return dbError(err)
	}
	return dbError(tx.Commit())
}
func (r *Repository) CreateImage(ctx context.Context, p domain.Image) (domain.Image, error) {
	p.CreatedAt = now()
	err := r.change(ctx, func(tx *sql.Tx) error {
		if err := insert(ctx, tx, "image", p.Uuid, "", p.Color, 0, p.CreatedAt, p); err != nil {
			return err
		}
		return enqueue(ctx, tx, "image", p)
	})
	return p, err
}
func (r *Repository) UpdateImage(ctx context.Context, id string, input domain.PhotoInput) (domain.Image, error) {
	var p domain.Image
	err := r.change(ctx, func(tx *sql.Tx) error {
		var err error
		p, err = scan[domain.Image](tx.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='image' AND id=?", id))
		if err != nil {
			return err
		}
		p.Name = input.Name
		p.Country = input.Country
		p.City = input.City
		p.YearTaken = input.YearTaken
		p.Color = input.Color
		raw, _ := json.Marshal(p)
		if _, err = tx.ExecContext(ctx, "UPDATE content SET color=?,payload=? WHERE kind='image' AND id=?", p.Color, raw, id); err != nil {
			return err
		}
		return enqueue(ctx, tx, "image", p)
	})
	return p, err
}
func (r *Repository) DeleteImage(ctx context.Context, id string) error {
	return r.change(ctx, func(tx *sql.Tx) error {
		p, err := scan[domain.Image](tx.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='image' AND id=?", id))
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM content WHERE kind='image' AND id=?", id); err != nil {
			return err
		}
		return enqueue(ctx, tx, "delete-image", p)
	})
}
func (r *Repository) CreatePost(ctx context.Context, p domain.Post) (domain.Post, error) {
	p.CreatedAt = now()
	p.UpdatedAt = p.CreatedAt
	err := r.change(ctx, func(tx *sql.Tx) error {
		if err := insert(ctx, tx, "post", p.ID, p.Slug, "", p.Published, p.CreatedAt, p); err != nil {
			return err
		}
		return enqueue(ctx, tx, "post", p)
	})
	return p, err
}
func (r *Repository) UpdatePost(ctx context.Context, id string, input domain.PostInput) (domain.Post, error) {
	var p domain.Post
	err := r.change(ctx, func(tx *sql.Tx) error {
		var err error
		p, err = scan[domain.Post](tx.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='post' AND id=?", id))
		if err != nil {
			return err
		}
		p.Slug = input.Slug
		p.Title = input.Title
		p.Excerpt = input.Excerpt
		p.Body = input.Body
		p.Published = input.Published
		p.UpdatedAt = now()
		raw, _ := json.Marshal(p)
		if _, err = tx.ExecContext(ctx, "UPDATE content SET slug=?,published=?,payload=? WHERE kind='post' AND id=?", p.Slug, p.Published, raw, id); err != nil {
			return err
		}
		return enqueue(ctx, tx, "post", p)
	})
	return p, err
}
func (r *Repository) DeletePost(ctx context.Context, id string) error {
	return r.change(ctx, func(tx *sql.Tx) error {
		p, err := scan[domain.Post](tx.QueryRowContext(ctx, "SELECT payload FROM content WHERE kind='post' AND id=?", id))
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM content WHERE kind='post' AND id=?", id); err != nil {
			return err
		}
		return enqueue(ctx, tx, "delete-post", p)
	})
}

// Initialize imports a consistent backup once. An empty but initialized database
// stays empty on restart; deleted records cannot be resurrected by stale backups.
func (r *Repository) Initialize(ctx context.Context, load func(context.Context) (domain.Snapshot, error)) error {
	var value string
	err := r.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='initialized'").Scan(&value)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	snapshot, err := load(ctx)
	if err != nil {
		return err
	}
	return r.change(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='initialized'").Scan(&value)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		for _, p := range snapshot.Images {
			if err := insert(ctx, tx, "image", p.Uuid, "", p.Color, 0, p.CreatedAt, p); err != nil {
				return err
			}
		}
		for _, p := range snapshot.Posts {
			if err := insert(ctx, tx, "post", p.ID, p.Slug, "", p.Published, p.CreatedAt, p); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES('initialized','1')")
		return err
	})
}
