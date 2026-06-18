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

type UpdateLetterTypeRequest struct {
	Code               string `json:"code"`
	Name               string `json:"name" binding:"required"`
	Description        string `json:"description"`
	WorkCode           string `json:"kode_kerja"`
	ClassificationCode string `json:"kode_klasifikasi"`
}

type LetterTypeResponse struct {
	ID                 uint   `json:"id"`
	Code               string `json:"code"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	WorkCode           string `json:"kode_kerja"`
	ClassificationCode string `json:"kode_klasifikasi"`
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

// UploadTemplateV2Request is received as multipart/form-data.
//
// Use cases:
//   - Create a new letter type and upload its template: omit letter_type_id and provide code + name (+ optional description).
//   - Replace/update template for an existing letter type: provide letter_type_id.
//     Optional: provide name and/or description to also update the letter type metadata.
//
// File is handled separately via ctx.FormFile("file").
type UploadTemplateV2Request struct {
	LetterTypeID       string `form:"letter_type_id"`
	Code               string `form:"code"`
	Name               string `form:"name"`
	Description        string `form:"description"`
	WorkCode           string `form:"kode_kerja"`
	ClassificationCode string `form:"kode_klasifikasi"`
}

type TemplateListItem struct {
	ID                  uint      `json:"id"`
	LetterTypeID        uint      `json:"letter_type_id"`
	Code                string    `json:"code"`
	Name                string    `json:"name"`
	Description         string    `json:"description"`
	WorkCode            string    `json:"kode_kerja"`
	ClassificationCode  string    `json:"kode_klasifikasi"`
	FilePath            string    `json:"file_path"`
	FileType            string    `json:"file_type"`
	CreatedBy           uint      `json:"created_by"`
	CreatorName         string    `json:"creator_name"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	Placeholders        []string  `json:"placeholders"`
	AutoFilledKeys      []string  `json:"auto_filled_keys"`
	RequiredPayloadKeys []string  `json:"required_payload_keys"`
}

type TemplateUploadV2Data struct {
	LetterTypeID        uint     `json:"letter_type_id"`
	Code                string   `json:"code"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	WorkCode            string   `json:"kode_kerja"`
	ClassificationCode  string   `json:"kode_klasifikasi"`
	FilePath            string   `json:"file_path"`
	Placeholders        []string `json:"placeholders"`
	AutoFilledKeys      []string `json:"auto_filled_keys"`
	RequiredPayloadKeys []string `json:"required_payload_keys"`
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
