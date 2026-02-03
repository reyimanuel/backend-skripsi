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
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/gorm"
)

var db *gorm.DB

func InitMiddleware(database *gorm.DB) {
	db = database
}

func MiddlewareLogin(ctx *gin.Context) {
	bearerToken := ctx.GetHeader("Authorization")

	if bearerToken == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
		ctx.Abort()
		return
	}

	tokenStr, errMsg := parseAuthHeader(bearerToken)
	if errMsg != "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errMsg})
		ctx.Abort()
		return
	}

	// Call ValidateAccessToken function from the 'token' package
	user, err := token.ValidateAccessToken(tokenStr)
	if err != nil {
		var errMsg string

		// Cek apakah error-nya expired
		if errors.Is(err, jwt.ErrTokenExpired) {
			errMsg = "Token has expired"
		} else if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			errMsg = "Invalid token signature"
		} else {
			errMsg = "Invalid or malformed token"
		}

		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errMsg})
		return
	}

	// Check if user still exists in DB
	if db != nil {
		var exists int64
		if err := db.Model(&migration.User{}).Where("id = ?", user.ID).Count(&exists).Error; err != nil {
			log.Printf("Middleware DB check error: %v", err)
			// Fail safe: if DB error, maybe allow? Or deny? Deny is safer.
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal server error during auth check"})
			return
		}
		if exists == 0 {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "User account no longer exists"})
			return
		}
	}

	ctx.Set("user", user)
	ctx.Next()
}

func MiddlewareRole(requiredRoles ...string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		user, exists := ctx.Get("user")
		if !exists {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		claims, ok := user.(*token.UserAuthToken)
		if !ok {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			return
		}

		log.Printf("role: %v", claims)

		authorized := slices.Contains(requiredRoles, claims.Role)

		if !authorized {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: insufficient role"})
			return
		}

		ctx.Next()
	}
}

// parseAuthHeader extracts the token from an Authorization header. Accepts both
// "Bearer <token>" and raw token formats to keep Swagger usage simple.
func parseAuthHeader(header string) (string, string) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", "Authorization header is required"
	}

	lower := strings.ToLower(header)
	if strings.HasPrefix(lower, "bearer ") {
		token := strings.TrimSpace(header[7:])
		if token == "" {
			return "", "Invalid token format"
		}
		return token, ""
	}

	if !strings.Contains(header, " ") {
		return header, ""
	}

	return "", "Invalid token format"
}
