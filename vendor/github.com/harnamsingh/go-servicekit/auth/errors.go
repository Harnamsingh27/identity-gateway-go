package auth

import (
	"github.com/harnamsingh/go-servicekit/errors"
)

// Sentinel errors returned by JWT and API-key middleware. They map through
// errors.ToHTTPStatus and errors.ToGRPCStatus to 401 Unauthorized.
var (
	// ErrMissingToken is returned when no Authorization header / metadata is present.
	ErrMissingToken = errors.New(errors.CodeUnauthorized, "missing token")

	// ErrExpiredToken is returned when the JWT exp claim is in the past.
	ErrExpiredToken = errors.New(errors.CodeUnauthorized, "token expired")

	// ErrInvalidSignature is returned when the JWT signature does not verify.
	ErrInvalidSignature = errors.New(errors.CodeUnauthorized, "invalid token signature")

	// ErrInvalidToken is returned for any other token parsing failure.
	ErrInvalidToken = errors.New(errors.CodeUnauthorized, "invalid token")

	// ErrMissingAPIKey is returned when no API key header is present.
	ErrMissingAPIKey = errors.New(errors.CodeUnauthorized, "missing API key")

	// ErrInvalidAPIKey is returned when the provided API key is not recognised.
	ErrInvalidAPIKey = errors.New(errors.CodeUnauthorized, "invalid API key")
)
