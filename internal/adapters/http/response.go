package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/basarsubasi/website/internal/domain"
)

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]string{"error": message})
}

func appError(w http.ResponseWriter, err error) {
	var invalid *domain.ValidationError
	var storage *domain.StorageError
	switch {
	case errors.As(err, &invalid):
		fail(w, http.StatusBadRequest, invalid.Error())
	case errors.Is(err, domain.ErrNotFound):
		fail(w, http.StatusNotFound, "Not found")
	case errors.Is(err, domain.ErrConflict):
		fail(w, http.StatusConflict, "Content changed or identifier already exists; reload and retry")
	case errors.Is(err, domain.ErrUncertain):
		log.Printf("write awaiting R2 confirmation: %v", err)
		fail(w, http.StatusServiceUnavailable, "Write outcome is uncertain; reload before retrying")
	case errors.Is(err, domain.ErrBusy):
		w.Header().Set("Retry-After", "1")
		fail(w, http.StatusServiceUnavailable, "Storage is busy; retry shortly")
	case errors.Is(err, domain.ErrUnavailable):
		fail(w, http.StatusServiceUnavailable, "Object storage is not configured")
	case errors.Is(err, domain.ErrTooLarge):
		fail(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.As(err, &storage):
		log.Printf("storage: %v", storage.Err)
		fail(w, http.StatusBadGateway, storage.Error())
	default:
		log.Printf("application: %v", err)
		fail(w, http.StatusInternalServerError, "Operation failed")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "Invalid JSON body (maximum 1 MiB)")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, http.StatusBadRequest, "Expected a single JSON object")
		return false
	}
	return true
}

func pagination(w http.ResponseWriter, r *http.Request, defaultLimit int64) (int64, int64, bool) {
	page, limit := int64(1), defaultLimit
	for key, target := range map[string]*int64{"page": &page, "limit": &limit} {
		if raw := r.URL.Query().Get(key); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n < 1 {
				fail(w, http.StatusBadRequest, "page and limit must be positive integers")
				return 0, 0, false
			}
			*target = n
		}
	}
	if limit > 100 || page > 1000000 {
		fail(w, http.StatusBadRequest, "Pagination exceeds maximum (limit 100, page 1000000)")
		return 0, 0, false
	}
	return page, limit, true
}
