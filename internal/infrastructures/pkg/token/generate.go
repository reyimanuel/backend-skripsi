package token

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// login related token
func GenerateToken(data *UserAuthToken) (string, error) {
	// Create a new token using RS256 algorithm
	token := jwt.New(jwt.SigningMethodRS256)
	
	// Set token claims
	claims := token.Claims.(jwt.MapClaims)
	claims["data"] = data
	claims["iss"] = "myApp"
	claims["iat"] = time.Now().Unix()
    claims["exp"] = time.Now().Add(time.Duration(jwtConfig.jwtLifeTime) * time.Second).Unix()

	// Sign the token and return
	return token.SignedString(jwtConfig.privateKey)
}

func GenerateRefreshToken(id int) (string, error) {
	token := jwt.New(jwt.SigningMethodRS256)

	claims := token.Claims.(jwt.MapClaims)
	claims["data"] = map[string]int{
		"id": id,
	}
	claims["iss"] = "myApp"
	claims["iat"] = time.Now().Unix()
    claims["exp"] = time.Now().Add(time.Duration(jwtConfig.jwtRefreshLifeTime) * time.Second).Unix()

	return token.SignedString(jwtConfig.privateKey)
}

// reset password token
func GenerateResetPasswordToken(user *UserAuthToken) (string, error) {
	token := jwt.New(jwt.SigningMethodRS256)

	claims := token.Claims.(jwt.MapClaims)
	claims["data"] = user
	claims["iss"] = "myApp"
	claims["iat"] = time.Now().Unix()
	claims["exp"] = time.Now().Add(15 * time.Minute).Unix()

	return token.SignedString(jwtConfig.privateKey)
}
