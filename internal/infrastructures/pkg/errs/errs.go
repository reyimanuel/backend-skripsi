package errs

import (
	"net/http"
)

// Response is a struct that represents the response structure for the API.
// It contains the status code, message, and data fields.
type MessageError interface {
	Status() int
	Error() string
	Message() string
}

// ErrorData is a struct that implements the MessageError interface.
// It contains the status code, error message, and additional message fields.
type ErrorData struct {
	ErrStatus  int    `json:"status"`
	ErrError   string `json:"error"`
	ErrMessage string `json:"message"`
}

// Status returns the status code of the response.
func (e *ErrorData) Status() int {
	return e.ErrStatus
}

// Error returns the error string.
func (e *ErrorData) Error() string {
	return e.ErrError
}

// Message returns a message associated with the error.
func (e *ErrorData) Message() string {
	return e.ErrMessage
}

// Client Error Responses (400s)
// BadRequest returns a MessageError representing a 400 Bad Request error with a custom message.
func BadRequest(message string) MessageError {
	return &ErrorData{
		ErrStatus:  http.StatusBadRequest,
		ErrError:   "Bad Request",
		ErrMessage: message,
	}
}

// Unauthorized returns a MessageError representing a 401 Unauthorized error with a custom message.
func Unauthorized(message string) MessageError {
	return &ErrorData{
		ErrMessage: message,
		ErrStatus:  http.StatusUnauthorized,
		ErrError:   "Unauthorized",
	}
}

// Forbidden returns a MessageError representing a 403 Forbidden error with a custom message.
func Forbidden(message string) MessageError {
	return &ErrorData{
		ErrMessage: message,
		ErrStatus:  http.StatusForbidden,
		ErrError:   "Forbidden",
	}
}

// NotFound returns a MessageError representing a 404 Not Found error with a custom message.
func NotFound(message string) MessageError {
	return &ErrorData{
		ErrMessage: message,
		ErrStatus:  http.StatusNotFound,
		ErrError:   "Not Found",
	}
}

// Client Error Responses (500s)
// InternalServerError returns a MessageError representing a 500 Internal Server Error with a custom message.
func InternalServerError(message string) MessageError {
	return &ErrorData{
		ErrMessage: message,
		ErrStatus:  http.StatusInternalServerError,
		ErrError:   "Internal Server Error",
	}
}
