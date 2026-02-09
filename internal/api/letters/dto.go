package letters

import (
	"encoding/json"
	"time"
)

type CreateLetterRequest struct {
	LetterTypeID uint            `json:"letter_type_id" binding:"required"`
	Subject      string          `json:"subject" binding:"required"`
	Payload      json.RawMessage `json:"payload" binding:"required"`
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
