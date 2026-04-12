package notifications

import "time"

type SendTestNotificationRequest struct {
	Title string            `json:"title" binding:"required"`
	Body  string            `json:"body" binding:"required"`
	Data  map[string]string `json:"data,omitempty"`
}

type SendResult struct {
	Tokens  int `json:"tokens"`
	Success int `json:"success"`
	Failure int `json:"failure"`
	Revoked int `json:"revoked"`
}

type Response struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Data       any    `json:"data,omitempty"`
}

type NotificationItem struct {
	ID        uint              `json:"id"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Data      map[string]string `json:"data"`
	IsRead    bool              `json:"is_read"`
	ReadAt    *time.Time        `json:"read_at,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}
