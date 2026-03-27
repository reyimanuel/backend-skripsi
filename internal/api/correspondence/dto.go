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
	PreviewURL   string         `json:"preview_url,omitempty"`
	Payload      map[string]any `json:"payload"`
	CreatedAt    time.Time      `json:"created_at"`
}

type PreviewResponse struct {
	ID         uint   `json:"id"`
	PreviewURL string `json:"preview_url"`
}

type Response struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Data       any    `json:"data,omitempty"`
}

type AttachmentItem struct {
	ID       uint   `json:"id"`
	FilePath string `json:"file_path"`
	FileType string `json:"file_type,omitempty"`
}

type UploadAttachmentsResponse struct {
	LetterID    uint             `json:"letter_id"`
	Attachments []AttachmentItem `json:"attachments"`
}

type ApproveLetterRequest struct {
	Action       string `json:"action" binding:"required,oneof=approve reject forward"`
	SignedByRole string `json:"signed_by_role"` // dekan | wakil_dekan
	Notes        string `json:"notes"`
}

type HistoryActor struct {
	UserID uint   `json:"user_id"`
	Name   string `json:"name"`
}

type LetterHistoryItem struct {
	ID        uint          `json:"id"`
	Action    string        `json:"action"`
	Notes     string        `json:"notes"`
	Actor     *HistoryActor `json:"actor,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
}

type ListLettersQuery struct {
	Q           string `form:"q"`
	Status      string `form:"status"`
	LetterType  uint   `form:"letter_type_id"`
	CreatedFrom string `form:"created_from"`
	CreatedTo   string `form:"created_to"`
	Sort        string `form:"sort"`
	Page        int    `form:"page"`
	PageSize    int    `form:"page_size"`
}

type LetterTypeSummary struct {
	ID   uint   `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type StudentSummary struct {
	StudentID uint   `json:"student_id"`
	UserID    uint   `json:"user_id"`
	Name      string `json:"name"`
	NIM       string `json:"nim"`
}

type LetterListItem struct {
	ID         uint              `json:"id"`
	Subject    string            `json:"subject"`
	Status     string            `json:"status"`
	LetterNo   *string           `json:"letter_number,omitempty"`
	LetterType LetterTypeSummary `json:"letter_type"`
	Student    *StudentSummary   `json:"student,omitempty"`
	PreviewURL string            `json:"preview_url"`
	HistoryURL string            `json:"history_url"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type PaginationMeta struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

type LetterListData struct {
	Items []LetterListItem `json:"items"`
	Meta  PaginationMeta   `json:"meta"`
}
