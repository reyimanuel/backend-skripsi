package errs

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
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

// ErrorDataWithPayload is like ErrorData but can include additional structured data.
// Useful for validation responses (e.g. missing required fields) without changing
// the global error response shape for other endpoints.
type ErrorDataWithPayload struct {
	ErrStatus  int    `json:"status"`
	ErrError   string `json:"error"`
	ErrMessage string `json:"message"`
	Data       any    `json:"data,omitempty"`
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

func (e *ErrorDataWithPayload) Status() int {
	return e.ErrStatus
}

func (e *ErrorDataWithPayload) Error() string {
	return e.ErrError
}

func (e *ErrorDataWithPayload) Message() string {
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

// BadRequestWithData returns a 400 Bad Request with extra payload.
func BadRequestWithData(message string, data any) MessageError {
	return &ErrorDataWithPayload{
		ErrStatus:  http.StatusBadRequest,
		ErrError:   "Bad Request",
		ErrMessage: message,
		Data:       data,
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

// handlerError is a helper function to handle errors in the controller.
// It checks if the error is of type MessageError and responds with the appropriate status code and message.
func HandlerError(ctx *gin.Context, err error) {
	var messageErr MessageError
	if errors.As(err, &messageErr) {
		if messageErr.Status() >= http.StatusInternalServerError {
			log.Printf("internal error: method=%s path=%s status=%d err=%v", ctx.Request.Method, ctx.FullPath(), messageErr.Status(), err)
			ctx.JSON(http.StatusInternalServerError, InternalServerError("Terjadi gangguan pada server. Silakan coba lagi."))
			return
		}
		ctx.JSON(messageErr.Status(), messageErr)
		return
	}
	_ = ctx.Error(err).SetType(gin.ErrorTypePrivate) // record internal error
	log.Printf("unhandled error: method=%s path=%s err=%v", ctx.Request.Method, ctx.FullPath(), err)
	ctx.JSON(http.StatusInternalServerError, InternalServerError("Terjadi gangguan pada server. Silakan coba lagi."))
}
