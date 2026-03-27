package letters

import (
	"encoding/json"
	"mime/multipart"
	"time"
)

type AttachmentRequirement struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Accept   string `json:"accept,omitempty"` // e.g. "application/pdf,image/*"
}

type UpdateAttachmentRequirementsRequest struct {
	Requirements []AttachmentRequirement `json:"requirements" binding:"required"`
}

type LetterTypeRequirementsResponse struct {
	LetterTypeID uint                    `json:"letter_type_id"`
	Code         string                  `json:"code"`
	Name         string                  `json:"name"`
	Requirements []AttachmentRequirement `json:"requirements"`
}

func (r LetterTypeRequirementsResponse) MarshalJSON() ([]byte, error) {
	type Alias LetterTypeRequirementsResponse
	if r.Requirements == nil {
		r.Requirements = []AttachmentRequirement{}
	}
	return json.Marshal(Alias(r))
}

type UploadTemplateRequest struct {
	File *multipart.FileHeader `form:"file" binding:"required"`
}

// UploadTemplateFlexibleRequest is received as multipart/form-data.
//
// Use cases:
// - Replace template for an existing letter type: provide letter_type_id.
// - Create a new letter type and upload its template: omit letter_type_id and provide code + name (+ optional description).
//
// File is handled separately via ctx.FormFile("file").
type UploadTemplateFlexibleRequest struct {
	LetterTypeID string `form:"letter_type_id"`
	Code         string `form:"code"`
	Name         string `form:"name"`
	Description  string `form:"description"`
}

type TemplateListItem struct {
	ID           uint      `json:"id"`
	LetterTypeID uint      `json:"letter_type_id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	FilePath     string    `json:"file_path"`
	FileType     string    `json:"file_type"`
	CreatedBy    uint      `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
