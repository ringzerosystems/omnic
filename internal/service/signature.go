package service

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // Twilio HMAC-SHA1 signature specification requirement
	"encoding/base64"
	"sort"
	"strings"
)

// CalculateSignature calculates the X-Twilio-Signature header using HMAC-SHA1.
func CalculateSignature(targetURL string, params map[string]string, authToken string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(targetURL)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}

	mac := hmac.New(sha1.New, []byte(authToken))
	mac.Write([]byte(sb.String()))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// ValidateSignature verifies an incoming X-Twilio-Signature against expected parameters.
func ValidateSignature(targetURL string, params map[string]string, authToken, signature string) bool {
	expected := CalculateSignature(targetURL, params, authToken)
	return hmac.Equal([]byte(expected), []byte(signature))
}
