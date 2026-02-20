package letters

import (
	"mime/multipart"
	"time"
)

type UploadTemplateRequest struct {
	File *multipart.FileHeader `form:"file" binding:"required"`
}

type Data struct {
	ID           uint      `json:"id"`
	LetterTypeID uint      `json:"letter_type_id"`
	Subject      string    `json:"subject"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

type Response struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Data       any    `json:"data,omitempty"`
}
