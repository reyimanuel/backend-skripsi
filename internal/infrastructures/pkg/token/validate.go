package token

import (
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/utils"
)

type UserAuthToken struct {
	UserID uint     `json:"user_id"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

type StaffInvitationToken struct {
	UserID   uint   `json:"user_id"`
	Email    string `json:"email"`
	RoleCode string `json:"role_code"`
}

type StudentInvitationToken struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	NIM    string `json:"nim"`
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

func ValidateStaffInvitationToken(tokenStr string) (*StaffInvitationToken, error) {
	tkn, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtConfig.publicKey, nil
	})
	if err != nil || !tkn.Valid {
		return nil, errors.New("invalid staff invitation token")
	}

	claims, ok := tkn.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid staff invitation claims")
	}

	if typ, _ := claims["typ"].(string); typ != "staff_invitation" {
		return nil, errors.New("invalid staff invitation type")
	}

	email, _ := claims["email"].(string)
	roleCode, _ := claims["role_code"].(string)
	sub, ok := claims["sub"].(float64)
	if !ok || strings.TrimSpace(email) == "" || strings.TrimSpace(roleCode) == "" {
		return nil, errors.New("invalid staff invitation claims")
	}

	return &StaffInvitationToken{
		UserID:   uint(sub),
		Email:    email,
		RoleCode: strings.ToUpper(strings.TrimSpace(roleCode)),
	}, nil
}

func ValidateStudentInvitationToken(tokenStr string) (*StudentInvitationToken, error) {
	tkn, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtConfig.publicKey, nil
	})
	if err != nil || !tkn.Valid {
		return nil, errors.New("invalid student invitation token")
	}

	claims, ok := tkn.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid student invitation claims")
	}

	if typ, _ := claims["typ"].(string); typ != "student_invitation" {
		return nil, errors.New("invalid student invitation type")
	}

	email, _ := claims["email"].(string)
	nim, _ := claims["nim"].(string)
	sub, ok := claims["sub"].(float64)
	if !ok || strings.TrimSpace(email) == "" || strings.TrimSpace(nim) == "" {
		return nil, errors.New("invalid student invitation claims")
	}

	return &StudentInvitationToken{
		UserID: uint(sub),
		Email:  email,
		NIM:    strings.TrimSpace(nim),
	}, nil
}
