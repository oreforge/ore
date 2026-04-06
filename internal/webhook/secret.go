package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

func DeriveSecret(token, projectName string) string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte(projectName))
	return hex.EncodeToString(mac.Sum(nil))
}

func ValidateSecret(token, projectName, provided string) bool {
	expected := DeriveSecret(token, projectName)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}
