package token

import (
	"crypto/rsa"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/config"

	"github.com/golang-jwt/jwt/v5"
)

// jwtConfig holds the loaded JWT configuration and RSA keys used for signing and verifying tokens.
// It is initialized by the Load() function and accessed by other token-related functions in this package.
var jwtConfig *jwtStruct

type jwtStruct struct {
	jwtLifeTime        int64
	jwtRefreshLifeTime int64
	privateKey         *rsa.PrivateKey
	publicKey          *rsa.PublicKey
}

func ValidateKeyPath(path string) (string, error) {
	// Clean the path to resolve any .. or . components
	CleanPath := filepath.Clean(path)

	if !strings.HasSuffix(CleanPath, ".pem") {
		panic(fmt.Errorf("unsafe input"))
	}

	return CleanPath, nil
}

// Load reads RSA public and private keys from configured file paths,
// parses them, and sets the global jwtConfig variable with the loaded keys and token lifetimes.
func Load() {
	cfg := config.Get()

	publicKeyPath, err := ValidateKeyPath(cfg.PublicKeyPath)
	if err != nil {
		log.Fatalf("Error validating public key path: %v", err)
	}

	privateKeyPath, err := ValidateKeyPath(cfg.PrivateKeyPath)
	if err != nil {
		log.Fatalf("Error validating private key path: %v", err)
	}

	publicKeyFile, err := os.ReadFile(filepath.Clean(publicKeyPath))
	if err != nil {
		log.Fatalf("Error reading public key file: %v", err)
	}

	publicKey, err := jwt.ParseRSAPublicKeyFromPEM(publicKeyFile)
	if err != nil {
		log.Fatalf("Error parsing public key: %v", err)
	}

	privateKeyFile, err := os.ReadFile(filepath.Clean(privateKeyPath))
	if err != nil {
		log.Fatalf("Error reading private key file: %v", err)
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyFile)
	if err != nil {
		log.Fatalf("Error parsing private key: %v", err)
	}

	jwtConfig = &jwtStruct{
		jwtLifeTime:        cfg.AccessTokenLifeTime,
		jwtRefreshLifeTime: cfg.RefreshTokenLifeTime,
		publicKey:          publicKey,
		privateKey:         privateKey,
	}
}
