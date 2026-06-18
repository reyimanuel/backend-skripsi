package events

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/token"
	"github.com/reyimanuel/letter-administration/internal/realtime"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Stream(ctx *gin.Context) {
	tokenValue := strings.TrimSpace(ctx.Query("token"))
	if tokenValue == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"status_code": http.StatusUnauthorized,
			"message":     "Token SSE wajib diisi",
		})
		return
	}

	if _, err := token.ValidateAccessToken(tokenValue); err != nil {
		message := "Token SSE tidak valid"
		if errors.Is(err, jwt.ErrTokenExpired) {
			message = "Token SSE sudah kedaluwarsa"
		}
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"status_code": http.StatusUnauthorized,
			"message":     message,
		})
		return
	}

	flusher, ok := ctx.Writer.(http.Flusher)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"status_code": http.StatusInternalServerError,
			"message":     "Streaming tidak didukung",
		})
		return
	}

	ctx.Writer.Header().Set("Content-Type", "text/event-stream")
	ctx.Writer.Header().Set("Cache-Control", "no-cache")
	ctx.Writer.Header().Set("Connection", "keep-alive")
	ctx.Writer.Header().Set("X-Accel-Buffering", "no")
	ctx.Writer.WriteHeader(http.StatusOK)

	client, unsubscribe := realtime.Subscribe()
	defer unsubscribe()

	writeEvent(ctx, "connected", `{"status":"connected"}`)
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case event, ok := <-client.Events():
			if !ok {
				return
			}
			writeEvent(ctx, "sitara.update", realtime.MarshalEvent(event))
			flusher.Flush()
		case <-heartbeat.C:
			writeEvent(ctx, "heartbeat", fmt.Sprintf(`{"at":%q}`, time.Now().UTC().Format(time.RFC3339Nano)))
			flusher.Flush()
		}
	}
}

func writeEvent(ctx *gin.Context, eventName string, data string) {
	if eventName != "" {
		_, _ = fmt.Fprintf(ctx.Writer, "event: %s\n", eventName)
	}
	_, _ = fmt.Fprintf(ctx.Writer, "data: %s\n\n", data)
}
