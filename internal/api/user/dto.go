package user

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginResponse struct {
	StatusCode   int    `json:"status_code"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}
