package httpadapter

import (
	"net/http"

	"github.com/basarsubasi/website/internal/module/blog/domain/article"
	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	adminPostsPath = "/api/admin/posts"
	slugParam      = "slug"
	postsDataField = "data"
	idParam        = "id"
)

func (s *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	page, limit, ok := pagination(w, r, 10)
	if !ok {
		return
	}
	data, err := s.blog.ListPosts(r.Context(), domain.Pagination{Page: page, Limit: limit}, r.URL.Path == adminPostsPath)
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusOK, data)
}

func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	data, err := s.blog.GetPost(r.Context(), r.PathValue(slugParam))
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{postsDataField: data})
}

func (s *Server) savePost(w http.ResponseWriter, r *http.Request) {
	var p article.PostInput
	if !decode(w, r, &p) {
		return
	}
	post, err := s.blog.SavePost(r.Context(), r.PathValue(idParam), p)
	if err != nil {
		appError(w, err)
		return
	}
	status := http.StatusOK
	if r.Method == http.MethodPost {
		status = http.StatusCreated
	}
	reply(w, status, map[string]any{postsDataField: post})
}

func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	if err := s.blog.DeletePost(r.Context(), r.PathValue(idParam)); err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusNoContent, nil)
}
