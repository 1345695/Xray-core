package websocket

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

const webSocketRefererAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

func TestEncodeWebSocketRefererEarlyData(t *testing.T) {
	host := "cdn.example.com"
	earlyData := []byte("hello websocket early data")
	encodedEarlyData := base64.RawURLEncoding.EncodeToString(earlyData)

	referer, err := encodeWebSocketRefererEarlyData(host, encodedEarlyData)
	if err != nil {
		t.Fatalf("encodeWebSocketRefererEarlyData failed: %v", err)
	}
	prefixLen := webSocketRefererPrefixLen(host)
	if got, want := len(referer), prefixLen+len(encodedEarlyData); got != want {
		t.Fatalf("unexpected referer length: got %d want %d", got, want)
	}
	for _, ch := range referer[:prefixLen] {
		if !strings.ContainsRune(webSocketRefererAlphabet, ch) {
			t.Fatalf("unexpected prefix character %q", ch)
		}
	}
	if referer[prefixLen:] != encodedEarlyData {
		t.Fatal("expected encoded early data to remain unchanged after the prefix")
	}

	decoded, err := decodeWebSocketRefererEarlyData(host, referer)
	if err != nil {
		t.Fatalf("decodeWebSocketRefererEarlyData failed: %v", err)
	}
	if !bytes.Equal(decoded, earlyData) {
		t.Fatalf("unexpected decoded early data: got %q want %q", decoded, earlyData)
	}
}

func TestDecodeWebSocketRefererEarlyDataRejectsShortReferer(t *testing.T) {
	if _, err := decodeWebSocketRefererEarlyData("long-enough-host", "short"); err == nil {
		t.Fatal("expected short referer to be rejected")
	}
}
