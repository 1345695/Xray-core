package websocket

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func encodeWebSocketRefererEarlyData(host string, encodedEarlyData string) (string, error) {
	prefix, err := newRandomWebSocketRefererPrefix(webSocketRefererPrefixLen(host))
	if err != nil {
		return "", err
	}

	return prefix + encodedEarlyData, nil
}

func decodeWebSocketRefererEarlyData(host string, str string) ([]byte, error) {
	normalized := replacer.Replace(str)
	prefixLen := webSocketRefererPrefixLen(host)
	if len(normalized) < prefixLen {
		return nil, fmt.Errorf("websocket referer early data shorter than random prefix: got %d want at least %d", len(normalized), prefixLen)
	}
	normalized = normalized[prefixLen:]

	return base64.RawURLEncoding.DecodeString(normalized)
}

func webSocketRefererPrefixLen(host string) int {
	return len(host)
}

func newRandomWebSocketRefererPrefix(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}

	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(randomBytes)[:length], nil
}
