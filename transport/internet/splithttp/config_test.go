package splithttp_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	. "github.com/xtls/xray-core/transport/internet/splithttp"
)

func Test_GetNormalizedPath(t *testing.T) {
	c := Config{
		Path: "/?world",
	}

	path := c.GetNormalizedPath()
	if path != "/" {
		t.Error("Unexpected: ", path)
	}
}

func TestFillStreamRequest_UsesCfPaddInReferer(t *testing.T) {
	c := Config{
		XPaddingBytes: &RangeConfig{
			From: 24,
			To:   24,
		},
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/sh", nil)
	if err != nil {
		t.Fatal(err)
	}

	c.FillStreamRequest(req, "", "")

	referrer := req.Header.Get("Referer")
	if referrer == "" {
		t.Fatal("expected Referer header")
	}

	referrerURL, err := url.Parse(referrer)
	if err != nil {
		t.Fatal(err)
	}

	paddingValue := referrerURL.Query().Get("cf_padd")
	if len(paddingValue) != 24 {
		t.Fatalf("unexpected cf_padd length: got %d want 24", len(paddingValue))
	}
	if referrerURL.Query().Get("x_padding") != "" {
		t.Fatal("legacy x_padding should not be present in Referer")
	}
	if !regexp.MustCompile(`^[0-9A-Za-z]+$`).MatchString(paddingValue) {
		t.Fatalf("cf_padd should be alphanumeric: %q", paddingValue)
	}
	if paddingValue == strings.Repeat("X", len(paddingValue)) {
		t.Fatalf("cf_padd should not fall back to fixed X padding: %q", paddingValue)
	}
}

func TestExtractXPaddingFromRequest_DefaultModeSupportsCfPaddAndLegacyKey(t *testing.T) {
	tests := []struct {
		name      string
		referer   string
		wantValue string
		wantKey   string
	}{
		{
			name:      "new key",
			referer:   "https://example.com/sh?cf_padd=abc123XYZ",
			wantValue: "abc123XYZ",
			wantKey:   "cf_padd",
		},
		{
			name:      "legacy key",
			referer:   "https://example.com/sh?x_padding=legacy123",
			wantValue: "legacy123",
			wantKey:   "x_padding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/sh", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Referer", tt.referer)

			c := Config{}
			gotValue, gotPlacement := c.ExtractXPaddingFromRequest(req, false)

			if gotValue != tt.wantValue {
				t.Fatalf("unexpected padding value: got %q want %q", gotValue, tt.wantValue)
			}
			if !strings.Contains(gotPlacement, tt.wantKey) {
				t.Fatalf("unexpected padding placement: %q", gotPlacement)
			}
		})
	}
}
