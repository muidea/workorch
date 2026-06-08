package workorchclient

import "fmt"

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("workorch API error (status %d): %s", e.StatusCode, e.Message)
}

func IsNotFound(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.StatusCode == 404
	}
	return false
}

func IsConflict(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.StatusCode == 409
	}
	return false
}

func IsTimeout(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.StatusCode == 408
	}
	return false
}
