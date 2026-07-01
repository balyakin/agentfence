package errorsx

import (
	"errors"
	"fmt"
)

type PublicError struct {
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	Details  map[string]any `json:"details"`
	ExitCode int            `json:"-"`
	Cause    error          `json:"-"`
}

func (e *PublicError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *PublicError) Unwrap() error {
	return e.Cause
}

func Wrap(code string, message string, exitCode int, cause error) *PublicError {
	return &PublicError{
		Code:     code,
		Message:  message,
		Details:  map[string]any{},
		ExitCode: exitCode,
		Cause:    cause,
	}
}

func WithDetails(err *PublicError, details map[string]any) *PublicError {
	if err == nil {
		return nil
	}
	err.Details = RedactDetails(details)
	return err
}

func IsPublic(err error) (*PublicError, bool) {
	var public *PublicError
	if errors.As(err, &public) {
		return public, true
	}
	return nil, false
}

func RedactDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(details))
	for key, value := range details {
		out[key] = redactValue(value)
	}
	return out
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case string:
		return redactString(typed)
	case []byte:
		return redactString(string(typed))
	case []string:
		copied := make([]string, len(typed))
		for i, item := range typed {
			copied[i] = redactString(item)
		}
		return copied
	case []any:
		copied := make([]any, len(typed))
		for i, item := range typed {
			copied[i] = redactValue(item)
		}
		return copied
	case map[string]any:
		return RedactDetails(typed)
	case map[string]string:
		copied := make(map[string]string, len(typed))
		for key, item := range typed {
			copied[key] = redactString(item)
		}
		return copied
	default:
		return value
	}
}

func redactString(value string) string {
	if len(value) > 64 {
		return "[redacted]"
	}
	return value
}
