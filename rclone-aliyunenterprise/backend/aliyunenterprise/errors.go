package aliyunenterprise

import (
	"errors"
	"fmt"
)

// typed provider errors, used for retry / fail-closed decisions
var (
	ErrAuth       = errors.New("aliyunenterprise: authentication error")
	ErrPermission = errors.New("aliyunenterprise: permission error")
	ErrNotFound   = errors.New("aliyunenterprise: not found")
	ErrRateLimited = errors.New("aliyunenterprise: rate limited")
	ErrTransient  = errors.New("aliyunenterprise: transient server error")
	ErrProtocol   = errors.New("aliyunenterprise: protocol error")
	ErrConsistency = errors.New("aliyunenterprise: consistency check failed")
	ErrCatalog    = errors.New("aliyunenterprise: local catalog error")
)

// isNotFound reports whether err represents an authoritative 404 (not-found).
func isNotFound(err error) bool {
	return err != nil && errors.Is(err, ErrNotFound)
}

// isRetryable reports whether the error is safe to retry without data danger.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrTransient) || errors.Is(err, ErrRateLimited)
}

// classifyHTTP turns a non-2xx HTTP status + provider code into a typed error.
func classifyHTTP(statusCode int, code, message string) error {
	detail := fmt.Sprintf("HTTP %d", statusCode)
	if code != "" {
		detail += " " + code
	}
	if message != "" {
		detail += ": " + message
	}
	switch {
	case statusCode == 401 || statusCode == 403:
		return fmt.Errorf("%w (%s)", ErrPermission, detail)
	case statusCode == 404:
		return fmt.Errorf("%w (%s)", ErrNotFound, detail)
	case statusCode == 429:
		return fmt.Errorf("%w (%s)", ErrRateLimited, detail)
	case statusCode >= 500:
		return fmt.Errorf("%w (%s)", ErrTransient, detail)
	default:
		return fmt.Errorf("%w (%s)", ErrProtocol, detail)
	}
}