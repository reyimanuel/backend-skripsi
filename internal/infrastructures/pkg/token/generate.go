package token

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// login related token
func GenerateToken(userID uint, email string, roles []string) (string, error) {
	// Create a new token using RS256 algorithm
	token := jwt.New(jwt.SigningMethodRS256)

	// Set token claims
	claims := token.Claims.(jwt.MapClaims)
	claims["sub"] = userID
	claims["email"] = email
	claims["roles"] = roles
	claims["iss"] = "myApp"
	claims["iat"] = time.Now().Unix()
	claims["exp"] = time.Now().Add(time.Duration(jwtConfig.jwtLifeTime) * time.Second).Unix()

	// Sign the token and return
	return token.SignedString(jwtConfig.privateKey)
}

func GenerateRefreshToken(userID uint) (string, error) {
	token := jwt.New(jwt.SigningMethodRS256)

	claims := token.Claims.(jwt.MapClaims)
	claims["sub"] = userID
	claims["iss"] = "myApp"
	claims["iat"] = time.Now().Unix()
	claims["exp"] = time.Now().Add(time.Duration(jwtConfig.jwtRefreshLifeTime) * time.Second).Unix()

	return token.SignedString(jwtConfig.privateKey)
}

// reset password token
func GenerateResetPasswordToken(userID uint, email string) (string, error) {
	token := jwt.New(jwt.SigningMethodRS256)

	claims := token.Claims.(jwt.MapClaims)
	claims["sub"] = userID
	claims["email"] = email
	claims["iss"] = "myApp"
	claims["iat"] = time.Now().Unix()
	claims["exp"] = time.Now().Add(15 * time.Minute).Unix()

	return token.SignedString(jwtConfig.privateKey)
}

func GenerateStaffInvitationToken(userID uint, email string, roleCode string) (string, error) {
	token := jwt.New(jwt.SigningMethodRS256)

	claims := token.Claims.(jwt.MapClaims)
	claims["sub"] = userID
	claims["email"] = email
	claims["role_code"] = roleCode
	claims["typ"] = "staff_invitation"
	claims["iss"] = "myApp"
	claims["iat"] = time.Now().Unix()
	claims["exp"] = time.Now().Add(72 * time.Hour).Unix()

	return token.SignedString(jwtConfig.privateKey)
}

func GenerateStudentInvitationToken(userID uint, email string, nim string) (string, error) {
	token := jwt.New(jwt.SigningMethodRS256)

	claims := token.Claims.(jwt.MapClaims)
	claims["sub"] = userID
	claims["email"] = email
	claims["nim"] = nim
	claims["typ"] = "student_invitation"
	claims["iss"] = "myApp"
	claims["iat"] = time.Now().Unix()
	claims["exp"] = time.Now().Add(72 * time.Hour).Unix()

	return token.SignedString(jwtConfig.privateKey)
}
