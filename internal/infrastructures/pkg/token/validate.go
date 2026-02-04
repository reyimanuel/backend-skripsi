package token

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/utils"
)

type UserAuthToken struct {
	UserID uint     `json:"user_id"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

// ValidateAccessToken parses and validates a JWT access token string,
// returning the UserAuthToken if valid, or an error if invalid.
func ValidateAccessToken(tokenStr string) (*UserAuthToken, error) {
	tkn, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtConfig.publicKey, nil
	})
	if err != nil || !tkn.Valid {
		return nil, errors.New("invalid token")
	}

	claims := tkn.Claims.(jwt.MapClaims)

	return &UserAuthToken{
		UserID: uint(claims["sub"].(float64)),
		Email:  claims["email"].(string),
		Roles:  utils.ToStringSlice(claims["roles"]),
	}, nil
}

// ValidateRefreshToken parses and validates a JWT refresh token string,
// returning the user ID if valid, or an error if invalid.
func ValidateRefreshToken(tokenStr string) (uint, error) {
	tkn, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtConfig.publicKey, nil
	})
	if err != nil || !tkn.Valid {
		return 0, errors.New("invalid refresh token")
	}

	claims := tkn.Claims.(jwt.MapClaims)

	return uint(claims["sub"].(float64)), nil
}

// ValidateResetPasswordToken parses and validates a JWT reset password token string,
// returning the token data if valid, or an error if invalid.
func ValidateResetPasswordToken(tokenStr string) (uint, error) {
	tkn, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		return jwtConfig.publicKey, nil
	})
	if err != nil || !tkn.Valid {
		return 0, errors.New("invalid reset token")
	}

	claims := tkn.Claims.(jwt.MapClaims)
	return uint(claims["sub"].(float64)), nil
}
