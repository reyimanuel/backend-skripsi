package events

import "github.com/gin-gonic/gin"

func RegisterRoutes(r *gin.RouterGroup) {
	handler := NewHandler()
	r.GET("/stream", handler.Stream)
}
