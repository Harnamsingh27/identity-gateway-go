// Package errors defines the typed application error used across go-servicekit,
// along with helpers that map errors to HTTP status codes and gRPC statuses.
package errors

import (
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorCode is a machine-readable string that identifies the class of error.
type ErrorCode string

const (
	// CodeNotFound indicates the requested resource does not exist.
	CodeNotFound ErrorCode = "NOT_FOUND"
	// CodeUnauthorized indicates the caller is not authenticated.
	CodeUnauthorized ErrorCode = "UNAUTHORIZED"
	// CodeForbidden indicates the caller is authenticated but lacks permission.
	CodeForbidden ErrorCode = "FORBIDDEN"
	// CodeInvalidArgument indicates a caller-supplied value is invalid.
	CodeInvalidArgument ErrorCode = "INVALID_ARGUMENT"
	// CodeAlreadyExists indicates the resource already exists.
	CodeAlreadyExists ErrorCode = "ALREADY_EXISTS"
	// CodeDeadlineExceeded indicates an operation timed out.
	CodeDeadlineExceeded ErrorCode = "DEADLINE_EXCEEDED"
	// CodeUnavailable indicates the service is temporarily unavailable.
	CodeUnavailable ErrorCode = "UNAVAILABLE"
	// CodeInternal indicates an unexpected server-side error.
	CodeInternal ErrorCode = "INTERNAL"
)

// AppError is the canonical application error type. It carries a machine-readable
// Code, a human-readable Message, and an optional wrapped Cause.
type AppError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the wrapped cause, enabling errors.Is/As traversal.
func (e *AppError) Unwrap() error {
	return e.Cause
}

// New creates an AppError with the given code and message.
func New(code ErrorCode, msg string) *AppError {
	return &AppError{Code: code, Message: msg}
}

// Newf creates an AppError with a formatted message.
func Newf(code ErrorCode, format string, args ...any) *AppError {
	return &AppError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap creates an AppError that wraps an underlying cause.
func Wrap(code ErrorCode, msg string, cause error) *AppError {
	return &AppError{Code: code, Message: msg, Cause: cause}
}

// Is reports whether target matches this error's code. This allows
// errors.Is(err, &AppError{Code: CodeNotFound}) to work.
func (e *AppError) Is(target error) bool {
	var t *AppError
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

// ToHTTPStatus maps an error to an HTTP status code. If the error is not an
// *AppError, 500 Internal Server Error is returned.
func ToHTTPStatus(err error) int {
	var ae *AppError
	if !errors.As(err, &ae) {
		return http.StatusInternalServerError
	}
	switch ae.Code {
	case CodeNotFound:
		return http.StatusNotFound
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeAlreadyExists:
		return http.StatusConflict
	case CodeDeadlineExceeded:
		return http.StatusGatewayTimeout
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// ToGRPCStatus maps an error to a *status.Status. If the error is not an
// *AppError, codes.Internal is used.
func ToGRPCStatus(err error) *status.Status {
	var ae *AppError
	if !errors.As(err, &ae) {
		return status.New(codes.Internal, err.Error())
	}
	c := grpcCode(ae.Code)
	return status.New(c, ae.Message)
}

func grpcCode(code ErrorCode) codes.Code {
	switch code {
	case CodeNotFound:
		return codes.NotFound
	case CodeUnauthorized, CodeForbidden:
		return codes.PermissionDenied
	case CodeInvalidArgument:
		return codes.InvalidArgument
	case CodeAlreadyExists:
		return codes.AlreadyExists
	case CodeDeadlineExceeded:
		return codes.DeadlineExceeded
	case CodeUnavailable:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}
