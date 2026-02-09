package letters

import "encoding/json"

type CreateLetterRequest struct {
	LetterTypeID uint            `json:"letter_type_id" binding:"required"`
	Subject      string          `json:"subject" binding:"required"`
	Payload      json.RawMessage `json:"payload" binding:"required"`
}
