package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// errTrailingData is returned when the body holds more than one JSON value.
var errTrailingData = errors.New("trailing data after JSON body")

// fieldTypeHints describes the expected JSON type of each request field, so
// type errors can say what the client should send instead of exposing Go
// type names.
var fieldTypeHints = map[string]string{
	"name":   "a string",
	"status": "an integer (0 or 1)",
}

func init() {
	// Make validator report JSON field names ("name") instead of Go struct
	// field names ("Name").
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				return ""
			}
			return name
		})
	}
}

// bindJSON decodes exactly one JSON value from the request body into obj and
// runs the binding validators. Unlike gin's ShouldBindJSON it rejects input
// with anything other than whitespace after the first JSON value.
func bindJSON(c *gin.Context, obj any) error {
	dec := json.NewDecoder(c.Request.Body)
	if err := dec.Decode(obj); err != nil {
		return err
	}

	var maxBytesErr *http.MaxBytesError
	if err := dec.Decode(&struct{}{}); errors.As(err, &maxBytesErr) {
		return err
	} else if !errors.Is(err, io.EOF) {
		return errTrailingData
	}

	return binding.Validator.ValidateStruct(obj)
}

// bindErrorResponse converts a binding error into an HTTP status and a
// client-facing message that names the offending field and never leaks Go
// internals.
func bindErrorResponse(err error) (int, ErrorResponse) {
	var (
		maxBytesErr    *http.MaxBytesError
		typeErr        *json.UnmarshalTypeError
		syntaxErr      *json.SyntaxError
		validationErrs validator.ValidationErrors
	)

	switch {
	case errors.As(err, &maxBytesErr):
		return http.StatusRequestEntityTooLarge,
			ErrorResponse{Error: fmt.Sprintf("request body must not exceed %d bytes", maxBytesErr.Limit)}
	case errors.Is(err, io.EOF):
		return http.StatusBadRequest, ErrorResponse{Error: "request body is required"}
	case errors.Is(err, errTrailingData):
		return http.StatusBadRequest, ErrorResponse{Error: "request body must contain a single JSON object"}
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return http.StatusBadRequest, ErrorResponse{Error: "request body must be valid JSON"}
	case errors.As(err, &typeErr):
		if typeErr.Field == "" {
			return http.StatusBadRequest, ErrorResponse{Error: "request body must be a JSON object"}
		}
		hint, ok := fieldTypeHints[typeErr.Field]
		if !ok {
			return http.StatusBadRequest, ErrorResponse{Error: typeErr.Field + " has an invalid type", Field: typeErr.Field}
		}
		return http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("%s must be %s", typeErr.Field, hint), Field: typeErr.Field}
	case errors.As(err, &validationErrs) && len(validationErrs) > 0:
		fe := validationErrs[0]
		if fe.Tag() == "required" {
			return http.StatusBadRequest, ErrorResponse{Error: fe.Field() + " is required", Field: fe.Field()}
		}
		return http.StatusBadRequest, ErrorResponse{Error: fe.Field() + " is invalid", Field: fe.Field()}
	default:
		return http.StatusBadRequest, ErrorResponse{Error: "invalid request body"}
	}
}
