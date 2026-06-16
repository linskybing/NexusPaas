package platform

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const (
	secretHashPrefix   = "pbkdf2-sha256"
	legacySHA256Prefix = "sha256:"
	secretIterations   = 210000
)

func HashSecret(secret string) string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		sum := sha256.Sum256([]byte(secret))
		return legacySHA256Prefix + hex.EncodeToString(sum[:])
	}
	derived, err := pbkdf2.Key(sha256.New, secret, salt, secretIterations, 32)
	if err != nil {
		sum := sha256.Sum256([]byte(secret))
		return legacySHA256Prefix + hex.EncodeToString(sum[:])
	}
	return fmt.Sprintf("%s:%d:%s:%s", secretHashPrefix, secretIterations, hex.EncodeToString(salt), hex.EncodeToString(derived))
}

func VerifySecret(hash, secret string) bool {
	switch {
	case strings.HasPrefix(hash, secretHashPrefix+":"):
		parts := strings.Split(hash, ":")
		if len(parts) != 4 {
			return false
		}
		iterations, err := strconv.Atoi(parts[1])
		if err != nil || iterations <= 0 {
			return false
		}
		salt, err := hex.DecodeString(parts[2])
		if err != nil {
			return false
		}
		want, err := hex.DecodeString(parts[3])
		if err != nil {
			return false
		}
		got, err := pbkdf2.Key(sha256.New, secret, salt, iterations, len(want))
		return err == nil && subtle.ConstantTimeCompare(got, want) == 1
	case strings.HasPrefix(hash, legacySHA256Prefix):
		sum := sha256.Sum256([]byte(secret))
		got := legacySHA256Prefix + hex.EncodeToString(sum[:])
		return subtle.ConstantTimeCompare([]byte(got), []byte(hash)) == 1
	case strings.HasPrefix(hash, "plain:"):
		return subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(hash, "plain:")), []byte(secret)) == 1
	case hash != "":
		return subtle.ConstantTimeCompare([]byte(hash), []byte(secret)) == 1
	default:
		return false
	}
}
