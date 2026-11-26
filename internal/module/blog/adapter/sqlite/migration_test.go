package sqlite_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	blogsqlite "github.com/basarsubasi/website/internal/module/blog/adapter/sqlite"
	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/platform/database"
	"github.com/basarsubasi/website/internal/shared/domain/pagination"
)

const (
	legacyFilename         = "legacy.db"
	sqliteDriver           = "sqlite"
	legacySchemaSQL        = `CREATE TABLE content(kind TEXT,id TEXT PRIMARY KEY,slug TEXT,color TEXT,published INTEGER,created_at TEXT,payload BLOB)`
	legacyID               = "legacy"
	legacyTitle            = "Legacy"
	summaryText            = "Summary"
	repeatBodyText         = "body"
	creationDate           = "2026-01-01"
	insertLegacyPostSQL    = "INSERT INTO content VALUES('post',?,?, '',1,?,?)"
	listMismatchFormat     = "list total=%d error=%v"
	emptyBody              = ""
	summaryMismatchFormat  = "summary mismatch: %+v"
	changedPostFailure     = "migration changed full post"
	editedSlug             = "edited"
	editedTitle            = "Edited"
	updatedSummary         = "New summary"
	updatedBody            = "New body"
	otherID                = "other"
	conflictingSlug        = "taken"
	otherTitle             = "Other"
	invalidTitle           = "Invalid"
	postBody               = "Body"
	missingConflictFailure = "expected conflict"
	draftBody              = "Draft"
	countSummarySQL        = "SELECT count(*) FROM post_summaries WHERE id=?"
	staleSummaryFailure    = "stale summary"

	databaseFilename             = "site.db"
	postID                       = "post"
	postTitle                    = "Title"
	bodyPadding                  = "x"
	corruptSourceSQL             = `DROP TRIGGER post_summary_update; UPDATE content SET payload='invalid JSON' WHERE id='post'`
	readBodyFailureFormat        = "public list read body: %+v %v"
	readableCorruptSourceFailure = "corrupt source unexpectedly readable"
)

func TestLegacyPostSummaryMigrationAndTransactionalUpdates(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), legacyFilename)
	db, err := sql.Open(sqliteDriver, path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(legacySchemaSQL)
	if err != nil {
		t.Fatal(err)
	}
	post := article.Post{ID: legacyID, Slug: legacyID, Title: legacyTitle, Excerpt: summaryText, Body: strings.Repeat(repeatBodyText, 128<<10), Published: 1, CreatedAt: creationDate, UpdatedAt: creationDate}
	raw, _ := json.Marshal(post)
	_, err = db.Exec(insertLegacyPostSQL, post.ID, post.Slug, post.CreatedAt, raw)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	repo, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	posts := blogsqlite.New(repo)
	check := func(want article.Post, total int64) {
		t.Helper()
		page, err := posts.ListPosts(ctx, pagination.Pagination{Page: 1, Limit: 10}, false)
		if err != nil || page.Total != total {
			t.Fatalf(listMismatchFormat, page.Total, err)
		}
		if total > 0 {
			want.Body = emptyBody
			if page.Data[0] != want {
				t.Fatalf(summaryMismatchFormat, page.Data[0])
			}
		}
	}
	check(post, 1)
	got, err := posts.GetPost(ctx, post.Slug)
	if err != nil || got != post {
		t.Fatal(changedPostFailure, err)
	}
	updated, err := posts.UpdatePost(ctx, post.ID, article.PostInput{Slug: editedSlug, Title: editedTitle, Excerpt: updatedSummary, Body: updatedBody, Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	check(updated, 1)
	// A failed conflicting write must roll back its summary too.
	_, err = posts.CreatePost(ctx, article.Post{ID: otherID, Slug: conflictingSlug, Body: otherTitle, Published: 0})
	if err != nil {
		t.Fatal(err)
	}
	_, err = posts.UpdatePost(ctx, post.ID, article.PostInput{Slug: conflictingSlug, Title: invalidTitle, Body: postBody, Published: 1})
	if err == nil {
		t.Fatal(missingConflictFailure)
	}
	check(updated, 1)
	_, err = posts.UpdatePost(ctx, post.ID, article.PostInput{Slug: editedSlug, Body: draftBody, Published: 0})
	if err != nil {
		t.Fatal(err)
	}
	check(article.Post{}, 0)
	if err = posts.DeletePost(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = repo.DB().QueryRow(countSummarySQL, post.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal(staleSummaryFailure, err)
	}
	repo.Close()
	repo, err = database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	posts = blogsqlite.New(repo)
	check(article.Post{}, 0)
}

func TestPublicListingDoesNotReadBodyPayload(t *testing.T) {
	ctx := context.Background()
	repo, err := database.Open(ctx, filepath.Join(t.TempDir(), databaseFilename))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	posts := blogsqlite.New(repo)
	_, err = posts.CreatePost(ctx, article.Post{ID: postID, Slug: postID, Title: postTitle, Body: strings.Repeat(bodyPadding, 512<<10), Published: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt only the source payload after disabling summary maintenance. A
	// summary query must still work without touching or decoding that payload.
	_, err = repo.DB().Exec(corruptSourceSQL)
	if err != nil {
		t.Fatal(err)
	}
	page, err := posts.ListPosts(ctx, pagination.Pagination{Page: 1, Limit: 10}, false)
	if err != nil || len(page.Data) != 1 || page.Data[0].Title != postTitle || page.Data[0].Body != emptyBody {
		t.Fatalf(readBodyFailureFormat, page, err)
	}
	if _, err = posts.GetPost(ctx, postID); err == nil {
		t.Fatal(readableCorruptSourceFailure)
	}
}
