package events

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reyimanuel/letter-administration/internal/realtime"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Stream(ctx *gin.Context) {
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
