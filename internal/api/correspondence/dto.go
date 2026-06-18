package correspondence

import (
	"time"
)

type CreateDraftRequest struct {
	LetterTypeID uint           `json:"letter_type_id" binding:"required"`
	Subject      string         `json:"subject" binding:"required"`
	Payload      map[string]any `json:"payload" binding:"required"`
}

type UpdateDraftRequest struct {
	Subject *string        `json:"subject"`
	Payload map[string]any `json:"payload"`
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
	Key      string `json:"key,omitempty"`
	FilePath string `json:"file_path"`
	FileType string `json:"file_type,omitempty"`
}

type UploadAttachmentsResponse struct {
	LetterID    uint             `json:"letter_id"`
	Attachments []AttachmentItem `json:"attachments"`
}

type ApproveLetterRequest struct {
	Action           string `json:"action" binding:"required,oneof=approve reject forward"`
	SignedByRole     string `json:"signed_by_role"` // compatibility field; official forwarding uses target_official_id
	TargetOfficialID uint   `json:"target_official_id"`
	LetterNumber     string `json:"letter_number"`
	Notes            string `json:"notes"`
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

type LetterHistoryDetail struct {
	ID           uint               `json:"id"`
	LetterTypeID uint               `json:"letter_type_id"`
	LetterType   *LetterTypeSummary `json:"letter_type,omitempty"`
	Subject      string             `json:"subject"`
	Status       string             `json:"status"`
	LetterNumber *string            `json:"letter_number,omitempty"`
	Payload      map[string]any     `json:"payload"`
	Attachments  []AttachmentItem   `json:"attachments"`
	Student      *StudentSummary    `json:"student,omitempty"`
	PreviewURL   string             `json:"preview_url"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

type LetterHistoryAndDetailData struct {
	Letter    LetterHistoryDetail `json:"letter"`
	Histories []LetterHistoryItem `json:"histories"`
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
	ID                 uint   `json:"id"`
	Code               string `json:"code"`
	Name               string `json:"name"`
	WorkCode           string `json:"kode_kerja"`
	ClassificationCode string `json:"kode_klasifikasi"`
}

type StudentSummary struct {
	StudentID uint   `json:"student_id"`
	UserID    uint   `json:"user_id"`
	Name      string `json:"name"`
	NIM       string `json:"nim"`
}

type OfficialTargetItem struct {
	ID       uint   `json:"id"`
	UserID   uint   `json:"user_id"`
	Name     string `json:"name"`
	RoleCode string `json:"role_code"`
	Jabatan  string `json:"jabatan"`
	NIP      string `json:"nip,omitempty"`
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
