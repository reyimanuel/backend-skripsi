package middleware

import (
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/token"
	"gorm.io/gorm"
)

var db *gorm.DB

func InitMiddleware(database *gorm.DB) {
	db = database
}

type Response struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Data       any    `json:"data,omitempty"`
}

func MiddlewareAuth(ctx *gin.Context) {
	bearerToken := ctx.GetHeader("Authorization")
	if bearerToken == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, Response{
			StatusCode: http.StatusUnauthorized,
			Message:    "Header Authorization wajib diisi",
		})
		ctx.Abort()
		return
	}

	tokenStr, errMsg := parseAuthHeader(bearerToken)
	if errMsg != "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, Response{
			StatusCode: http.StatusUnauthorized,
			Message:    errMsg,
		})
		ctx.Abort()
		return
	}

	// Call ValidateAccessToken function from the 'token' package
	user, err := token.ValidateAccessToken(tokenStr)
	if err != nil {
		var errMsg string

		// Cek apakah error-nya expired
		if errors.Is(err, jwt.ErrTokenExpired) {
			errMsg = "Sesi Anda sudah berakhir, silakan login kembali"
		} else if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			errMsg = "Token tidak valid"
		} else {
			errMsg = "Token tidak valid atau format token salah"
		}

		ctx.AbortWithStatusJSON(http.StatusUnauthorized, Response{
			StatusCode: http.StatusUnauthorized,
			Message:    errMsg,
		})
		ctx.Abort()
		return
	}

	ctx.Set("auth", user)
	ctx.Next()
}

func MiddlewareRole(requiredRoles ...string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		auth, exists := ctx.Get("auth")
		if !exists {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, Response{
				StatusCode: http.StatusUnauthorized,
				Message:    "Anda belum login",
			})
			return
		}

		claims := auth.(*token.UserAuthToken)

		for _, role := range claims.Roles {
			if slices.Contains(requiredRoles, role) {
				ctx.Next()
				return
			}
		}

		log.Printf("access denied: user_id=%d user_roles=%v required_roles=%v", claims.UserID, claims.Roles, requiredRoles)
		ctx.AbortWithStatusJSON(http.StatusForbidden, Response{
			StatusCode: http.StatusForbidden,
			Message:    "Anda tidak memiliki izin untuk mengakses resource ini",
		})
	}
}

// parseAuthHeader extracts the token from an Authorization header. Accepts both
// "Bearer <token>" and raw token formats to keep Swagger usage simple.
func parseAuthHeader(header string) (string, string) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", "Header Authorization wajib diisi"
	}

	lower := strings.ToLower(header)
	if strings.HasPrefix(lower, "bearer ") {
		token := strings.TrimSpace(header[7:])
		if token == "" {
			return "", "Format token tidak valid"
		}
		return token, ""
	}

	if !strings.Contains(header, " ") {
		return header, ""
	}

	return "", "Format token tidak valid"
}

func GetUserID(c *gin.Context) (uint, error) {
	auth, exists := c.Get("auth")
	if !exists {
		return 0, errors.New("user not authenticated")
	}

	claims, ok := auth.(*token.UserAuthToken)
	if !ok {
		return 0, errors.New("failed to extract user claims")
	}
	return claims.UserID, nil
}
