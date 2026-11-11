package httpadapter

import (
	"net/http"

	"github.com/basarsubasi/website/internal/domain"
)

func (s *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	page, limit, ok := pagination(w, r, 10)
	if !ok {
		return
	}
	data, err := s.service.ListPosts(r.Context(), domain.Pagination{Page: page, Limit: limit}, r.URL.Path == "/api/admin/posts")
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusOK, data)
}

func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	data, err := s.service.GetPost(r.Context(), r.PathValue("slug"))
	if err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"data": data})
}

func (s *Server) savePost(w http.ResponseWriter, r *http.Request) {
	var p domain.PostInput
	if !decode(w, r, &p) {
		return
	}
	post, err := s.service.SavePost(r.Context(), r.PathValue("id"), p)
	if err != nil {
		appError(w, err)
		return
	}
	status := http.StatusOK
	if r.Method == http.MethodPost {
		status = http.StatusCreated
	}
	reply(w, status, map[string]any{"data": post})
}

func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	if err := s.service.DeletePost(r.Context(), r.PathValue("id")); err != nil {
		appError(w, err)
		return
	}
	reply(w, http.StatusNoContent, nil)
}
