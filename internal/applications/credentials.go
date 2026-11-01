package applications

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

const apiKeyPrefix = "ns_"

func GenerateAPIKey() (string, error) {
	secret := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		return "", fmt.Errorf("generate API key: %w", err)
	}
	return apiKeyPrefix + base64.RawURLEncoding.EncodeToString(secret), nil
}

func HashAPIKey(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}
