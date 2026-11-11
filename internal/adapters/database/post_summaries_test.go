package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basarsubasi/website/internal/domain"
)

func TestLegacyPostSummaryMigrationAndTransactionalUpdates(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE content(kind TEXT,id TEXT PRIMARY KEY,slug TEXT,color TEXT,published INTEGER,created_at TEXT,payload BLOB)`)
	if err != nil {
		t.Fatal(err)
	}
	post := domain.Post{ID: "legacy", Slug: "legacy", Title: "Legacy", Excerpt: "Summary", Body: strings.Repeat("body", 128<<10), Published: 1, CreatedAt: "2026-01-01", UpdatedAt: "2026-01-01"}
	raw, _ := json.Marshal(post)
	_, err = db.Exec("INSERT INTO content VALUES('post',?,?, '',1,?,?)", post.ID, post.Slug, post.CreatedAt, raw)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	repo, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	check := func(want domain.Post, total int64) {
		t.Helper()
		page, err := repo.ListPosts(ctx, domain.Pagination{Page: 1, Limit: 10}, false)
		if err != nil || page.Total != total {
			t.Fatalf("list total=%d error=%v", page.Total, err)
		}
		if total > 0 {
			want.Body = ""
			if page.Data[0] != want {
				t.Fatalf("summary mismatch: %+v", page.Data[0])
			}
		}
	}
	check(post, 1)
	got, err := repo.GetPost(ctx, post.Slug)
	if err != nil || got != post {
		t.Fatal("migration changed full post", err)
	}
	updated, err := repo.UpdatePost(ctx, post.ID, domain.PostInput{Slug: "edited", Title: "Edited", Excerpt: "New summary", Body: "New body", Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	check(updated, 1)
	// A failed conflicting write must roll back its summary too.
	_, err = repo.CreatePost(ctx, domain.Post{ID: "other", Slug: "taken", Body: "Other", Published: 0})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.UpdatePost(ctx, post.ID, domain.PostInput{Slug: "taken", Title: "Invalid", Body: "Body", Published: 1})
	if err == nil {
		t.Fatal("expected conflict")
	}
	check(updated, 1)
	_, err = repo.UpdatePost(ctx, post.ID, domain.PostInput{Slug: "edited", Body: "Draft", Published: 0})
	if err != nil {
		t.Fatal(err)
	}
	check(domain.Post{}, 0)
	if err = repo.DeletePost(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = repo.db.QueryRow("SELECT count(*) FROM post_summaries WHERE id=?", post.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal("stale summary", err)
	}
	repo.Close()
	repo, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	check(domain.Post{}, 0)
}

func TestPublicListingDoesNotReadBodyPayload(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "site.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	_, err = repo.CreatePost(ctx, domain.Post{ID: "post", Slug: "post", Title: "Title", Body: strings.Repeat("x", 512<<10), Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt only the source payload after disabling summary maintenance. A
	// summary query must still work without touching or decoding that payload.
	_, err = repo.db.Exec(`DROP TRIGGER post_summary_update; UPDATE content SET payload='invalid JSON' WHERE id='post'`)
	if err != nil {
		t.Fatal(err)
	}
	page, err := repo.ListPosts(ctx, domain.Pagination{Page: 1, Limit: 10}, false)
	if err != nil || len(page.Data) != 1 || page.Data[0].Title != "Title" || page.Data[0].Body != "" {
		t.Fatalf("public list read body: %+v %v", page, err)
	}
	if _, err = repo.GetPost(ctx, "post"); err == nil {
		t.Fatal("corrupt source unexpectedly readable")
	}
}
