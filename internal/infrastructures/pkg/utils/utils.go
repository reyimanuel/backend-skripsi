package utils

import (
	"crypto/rand"
	"math/big"
)

// SafeCompareString securely compares two strings to prevent timing attacks.
// This function ensures that the comparison time remains constant regardless of differences in characters,
// making it useful for comparing sensitive data such as passwords or tokens.
func SafeCompareString(a, b string) bool {
	// If the lengths are different, return false immediately
	if len(a) != len(b) {
		return false
	}

	// Variable to store the result of bitwise comparisons
	var result byte = 0

	// Iterate through each character in the string and perform a bitwise XOR operation
	for i := range a {
		result |= a[i] ^ b[i] // If there is a difference, result will become non-zero
	}

	// If result remains 0, all characters are identical, return true
	return result == 0
}

func RandomString(length int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			// fallback ke 'a' jika terjadi error
			b[i] = 'a'
			continue
		}
		b[i] = letters[n.Int64()]
	}
	return string(b)
}

func ToStringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
