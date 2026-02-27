package correspondence

import (
	"time"
)

type SubmitLetterRequest struct {
	LetterTypeID uint           `json:"letter_type_id" binding:"required"`
	Subject      string         `json:"subject" binding:"required"`
	Payload      map[string]any `json:"payload" binding:"required"`
}

type Data struct {
	ID           uint           `json:"id"`
	LetterTypeID uint           `json:"letter_type_id"`
	Subject      string         `json:"subject"`
	Status       string         `json:"status"`
	FilePath     string         `json:"file_path"`
	Payload      map[string]any `json:"payload"`
	CreatedAt    time.Time      `json:"created_at"`
}

type Response struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Data       any    `json:"data,omitempty"`
}

type ApproveLetterRequest struct {
	Action       string `json:"action" binding:"required,oneof=approve reject forward"`
	SignedByRole string `json:"signed_by_role"` // dekan | wakil_dekan
	Notes        string `json:"notes"`
}
