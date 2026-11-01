package applications

import (
	"strings"
	"testing"
)

func TestGenerateAPIKeyAndHash(t *testing.T) {
	first, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	second, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	if !strings.HasPrefix(first, apiKeyPrefix) || first == second {
		t.Fatal("generated API keys must have the expected prefix and be unique")
	}
	firstHash := HashAPIKey(first)
	if firstHash != HashAPIKey(first) || firstHash == first {
		t.Fatal("API key hash must be stable and must not be the plaintext key")
	}
}
