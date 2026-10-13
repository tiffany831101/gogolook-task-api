package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These cases cover fallback branches that the current endpoints cannot
// reach, so they are exercised directly rather than over HTTP.
func TestBindErrorResponse(t *testing.T) {
	type sample struct {
		Count int `json:"count" binding:"min=1"`
	}
	validationErr := binding.Validator.ValidateStruct(&sample{Count: 0})
	require.Error(t, validationErr)

	tests := []struct {
		name       string
		err        error
		wantStatus int
		want       ErrorResponse
	}{
		{
			name:       "empty body",
			err:        io.EOF,
			wantStatus: http.StatusBadRequest,
			want:       ErrorResponse{Error: "request body is required"},
		},
		{
			name:       "truncated json",
			err:        io.ErrUnexpectedEOF,
			wantStatus: http.StatusBadRequest,
			want:       ErrorResponse{Error: "request body must be valid JSON"},
		},
		{
			name:       "type error on field without a hint",
			err:        &json.UnmarshalTypeError{Value: "string", Type: reflect.TypeOf(0), Field: "count"},
			wantStatus: http.StatusBadRequest,
			want:       ErrorResponse{Error: "count has an invalid type", Field: "count"},
		},
		{
			name:       "non-required validation rule uses json field name",
			err:        validationErr,
			wantStatus: http.StatusBadRequest,
			want:       ErrorResponse{Error: "count is invalid", Field: "count"},
		},
		{
			name:       "wrapped type error is still recognised",
			err:        fmt.Errorf("decode: %w", &json.UnmarshalTypeError{Value: "bool", Type: reflect.TypeOf(0), Field: "status"}),
			wantStatus: http.StatusBadRequest,
			want:       ErrorResponse{Error: "status must be an integer (0 or 1)", Field: "status"},
		},
		{
			name:       "body over the size limit",
			err:        &http.MaxBytesError{Limit: 10},
			wantStatus: http.StatusRequestEntityTooLarge,
			want:       ErrorResponse{Error: "request body must not exceed 10 bytes"},
		},
		{
			name:       "unknown error falls back to generic message",
			err:        errors.New("unexpected failure"),
			wantStatus: http.StatusBadRequest,
			want:       ErrorResponse{Error: "invalid request body"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, got := bindErrorResponse(tc.err)
			assert.Equal(t, tc.wantStatus, status)
			assert.Equal(t, tc.want, got)
		})
	}
}
