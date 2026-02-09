package letters

type Handler struct {
	Service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{Service: service}
}

// func (h *Handler) Login(ctx *gin.Context) {

// }
