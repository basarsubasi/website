package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/basarsubasi/website/internal/shared/domain"
)

const (
	contentTypeHeader = "Content-Type"
	jsonContentType   = "application/json; charset=utf-8"

	errorField = "error"

	notFoundMessage           = "Not found"
	conflictMessage           = "Content changed or identifier already exists; reload and retry"
	uncertainWriteLogFormat   = "write awaiting R2 confirmation: %v"
	uncertainWriteMessage     = "Write outcome is uncertain; reload before retrying"
	responseRetryAfterHeader  = "Retry-After"
	storageRetrySeconds       = "1"
	storageBusyMessage        = "Storage is busy; retry shortly"
	storageUnavailableMessage = "Object storage is not configured"
	storageErrorLogFormat     = "storage: %v"
	applicationErrorLogFormat = "application: %v"
	operationFailedMessage    = "Operation failed"

	invalidJSONMessage      = "Invalid JSON body (maximum 1 MiB)"
	singleJSONObjectMessage = "Expected a single JSON object"

	pageParam                  = "page"
	limitParam                 = "limit"
	responseEmptyString        = ""
	invalidPaginationMessage   = "page and limit must be positive integers"
	excessivePaginationMessage = "Pagination exceeds maximum (limit 100, page 1000000)"
)

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set(contentTypeHeader, jsonContentType)
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]string{errorField: message})
}

func appError(w http.ResponseWriter, err error) {
	var invalid *domain.ValidationError
	var storage *domain.StorageError
	switch {
	case errors.As(err, &invalid):
		fail(w, http.StatusBadRequest, invalid.Error())
	case errors.Is(err, domain.ErrNotFound):
		fail(w, http.StatusNotFound, notFoundMessage)
	case errors.Is(err, domain.ErrConflict):
		fail(w, http.StatusConflict, conflictMessage)
	case errors.Is(err, domain.ErrUncertain):
		log.Printf(uncertainWriteLogFormat, err)
		fail(w, http.StatusServiceUnavailable, uncertainWriteMessage)
	case errors.Is(err, domain.ErrBusy):
		w.Header().Set(responseRetryAfterHeader, storageRetrySeconds)
		fail(w, http.StatusServiceUnavailable, storageBusyMessage)
	case errors.Is(err, domain.ErrUnavailable):
		fail(w, http.StatusServiceUnavailable, storageUnavailableMessage)
	case errors.Is(err, domain.ErrTooLarge), errors.Is(err, domain.ErrVideoTooLarge), errors.Is(err, domain.ErrAudioTooLarge):
		fail(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.As(err, &storage):
		log.Printf(storageErrorLogFormat, storage.Err)
		fail(w, http.StatusBadGateway, storage.Error())
	default:
		log.Printf(applicationErrorLogFormat, err)
		fail(w, http.StatusInternalServerError, operationFailedMessage)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, http.StatusBadRequest, invalidJSONMessage)
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		fail(w, http.StatusBadRequest, singleJSONObjectMessage)
		return false
	}
	return true
}

func pagination(w http.ResponseWriter, r *http.Request, defaultLimit int64) (int64, int64, bool) {
	page, limit := int64(1), defaultLimit
	for key, target := range map[string]*int64{pageParam: &page, limitParam: &limit} {
		if raw := r.URL.Query().Get(key); raw != responseEmptyString {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n < 1 {
				fail(w, http.StatusBadRequest, invalidPaginationMessage)
				return 0, 0, false
			}
			*target = n
		}
	}
	if limit > 100 || page > 1000000 {
		fail(w, http.StatusBadRequest, excessivePaginationMessage)
		return 0, 0, false
	}
	return page, limit, true
}
