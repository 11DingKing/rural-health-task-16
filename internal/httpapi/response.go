package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"ruralhealth/internal/errorsx"
)

// ErrorResponse is the unified JSON error format.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PaginatedResponse wraps list responses with pagination metadata.
type PaginatedResponse struct {
	Items    []any `json:"items"`
	Total    int   `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func writeError(w http.ResponseWriter, err error) {
	code := "INTERNAL_ERROR"
	status := http.StatusInternalServerError

	switch {
	case errors.Is(err, errorsx.ErrNotFound):
		code = "NOT_FOUND"
		status = http.StatusNotFound
	case errors.Is(err, errorsx.ErrAlreadyExists):
		code = "ALREADY_EXISTS"
		status = http.StatusConflict
	case errors.Is(err, errorsx.ErrConflict):
		code = "CONFLICT"
		status = http.StatusConflict
	case errors.Is(err, errorsx.ErrInvalidTransition):
		code = "INVALID_TRANSITION"
		status = http.StatusConflict
	case errors.Is(err, errorsx.ErrValidation):
		code = "VALIDATION_ERROR"
		status = http.StatusBadRequest
	case errors.Is(err, errorsx.ErrIdempotentReplay):
		code = "IDEMPOTENT_REPLAY"
		status = http.StatusOK
	case errors.Is(err, errorsx.ErrFrozenForReview):
		code = "FROZEN"
		status = http.StatusConflict
	case errors.Is(err, errorsx.ErrNoUpstream):
		code = "NO_UPSTREAM"
		status = http.StatusServiceUnavailable
	case errors.Is(err, errorsx.ErrCircuitOpen):
		code = "CIRCUIT_OPEN"
		status = http.StatusServiceUnavailable
	case errors.Is(err, errorsx.ErrRuleNotActive):
		code = "RULE_NOT_ACTIVE"
		status = http.StatusConflict
	case errors.Is(err, errorsx.ErrDeadLetter):
		code = "DEAD_LETTER"
		status = http.StatusGone
	case errors.Is(err, errorsx.ErrForbidden):
		code = "FORBIDDEN"
		status = http.StatusForbidden
	case errors.Is(err, errorsx.ErrQuotaExceeded):
		code = "QUOTA_EXCEEDED"
		status = http.StatusTooManyRequests
	case errors.Is(err, errorsx.ErrSLABreached):
		code = "SLA_BREACHED"
		status = http.StatusConflict
	}

	writeJSON(w, status, ErrorResponse{
		Error: ErrorBody{Code: code, Message: err.Error()},
	})
}

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, data)
}

func writeCreated(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusCreated, data)
}

func writePaginated(w http.ResponseWriter, items []any, total, page, pageSize int) {
	writeJSON(w, http.StatusOK, PaginatedResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

func parseJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return errorsx.ErrValidation
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		return errorsx.Wrap(err, "invalid JSON body")
	}
	return nil
}

func parsePagination(r *http.Request) (int, int) {
	page := 1
	pageSize := 20
	if v := r.URL.Query().Get("page"); v != "" {
		if n := parseInt(v); n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n := parseInt(v); n > 0 && n <= 200 {
			pageSize = n
		}
	}
	return page, pageSize
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
