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

func TestGetNormalizedQuery_StripsPd(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "no pd",
			path: "/sh?foo=bar",
			want: "foo=bar",
		},
		{
			name: "strip random pd and keep others",
			path: "/sh?foo=bar&pd=random&ed=2048",
			want: "foo=bar&ed=2048",
		},
		{
			name: "preserve raw query encoding",
			path: "/?ed=8192&pd=zjxhttppd&ip=142.248.137.15:8443",
			want: "ed=8192&ip=142.248.137.15:8443",
		},
		{
			name: "only pd",
			path: "/sh?pd=random",
			want: "",
		},
		{
			name: "pd off",
			path: "/sh?pd=off&ed=2048",
			want: "ed=2048",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{Path: tt.path}
			if got := c.GetNormalizedQuery(); got != tt.want {
				t.Fatalf("unexpected query: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestFillStreamRequest_UsesDefaultPaddingKeyInReferer(t *testing.T) {
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

	paddingValue := referrerURL.Query().Get("x_padding")
	if len(paddingValue) != 24 {
		t.Fatalf("unexpected x_padding length: got %d want 24", len(paddingValue))
	}
	if !regexp.MustCompile(`^[0-9A-Za-z]+$`).MatchString(paddingValue) {
		t.Fatalf("x_padding should be alphanumeric: %q", paddingValue)
	}
	if paddingValue == strings.Repeat("X", len(paddingValue)) {
		t.Fatalf("x_padding should not fall back to fixed X padding: %q", paddingValue)
	}
}

func TestFillStreamRequest_UsesConfiguredPaddingKeyInReferer(t *testing.T) {
	c := Config{
		Path: "/sh?ed=8192&pd=custom_pad&ip=142.248.137.15:8443",
		XPaddingBytes: &RangeConfig{
			From: 24,
			To:   24,
		},
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/sh?ed=8192&ip=142.248.137.15:8443", nil)
	if err != nil {
		t.Fatal(err)
	}

	c.FillStreamRequest(req, "", "")

	referrer := req.Header.Get("Referer")
	referrerURL, err := url.Parse(referrer)
	if err != nil {
		t.Fatal(err)
	}

	paddingValue := referrerURL.Query().Get("custom_pad")
	if len(paddingValue) != 24 {
		t.Fatalf("unexpected custom padding length: got %d want 24", len(paddingValue))
	}
	if referrerURL.Query().Get("pd") != "" {
		t.Fatal("pd should not be present in Referer")
	}
	if referrerURL.Query().Get("ed") != "" {
		t.Fatalf("expected ed to be absent from Referer, got %q", referrerURL.Query().Get("ed"))
	}
	if referrerURL.Query().Get("ip") != "" {
		t.Fatalf("expected ip to be absent from Referer, got %q", referrerURL.Query().Get("ip"))
	}
	if req.URL.RawQuery != "ed=8192&ip=142.248.137.15:8443" {
		t.Fatalf("expected request query to stay unchanged, got %q", req.URL.RawQuery)
	}
	if strings.Contains(referrer, "ed=8192") || strings.Contains(referrer, "ip=142.248.137.15:8443") {
		t.Fatalf("expected Referer to contain only padding query, got %q", referrer)
	}
	if referrerURL.Path != "/" {
		t.Fatalf("expected Referer path to be root only, got %q", referrerURL.Path)
	}
	if strings.Contains(referrer, "/sh") {
		t.Fatalf("expected Referer not to contain original request path, got %q", referrer)
	}
}

func TestFillStreamRequest_UsesRandomPaddingKeyWhenConfigured(t *testing.T) {
	c := Config{
		Path: "/sh?pd=random",
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

	referrerURL, err := url.Parse(req.Header.Get("Referer"))
	if err != nil {
		t.Fatal(err)
	}

	query := referrerURL.Query()
	if len(query) != 1 {
		t.Fatalf("expected exactly one query key, got %d", len(query))
	}

	var paddingKey string
	var paddingValue string
	for key, values := range query {
		paddingKey = key
		if len(values) != 1 {
			t.Fatalf("expected one padding value for key %q, got %d", key, len(values))
		}
		paddingValue = values[0]
	}

	if paddingKey == "x_padding" {
		t.Fatalf("expected a random padding key, got %q", paddingKey)
	}
	if len(paddingKey) < 2 || len(paddingKey) > 64 {
		t.Fatalf("unexpected random padding key length: got %d", len(paddingKey))
	}
	if !regexp.MustCompile(`^[0-9A-Za-z]+$`).MatchString(paddingKey) {
		t.Fatalf("random padding key should be alphanumeric: %q", paddingKey)
	}
	if len(paddingValue) != 24 {
		t.Fatalf("unexpected random padding value length: got %d want 24", len(paddingValue))
	}
	if !regexp.MustCompile(`^[0-9A-Za-z]+$`).MatchString(paddingValue) {
		t.Fatalf("random padding value should be alphanumeric: %q", paddingValue)
	}
	if paddingValue == strings.Repeat("X", len(paddingValue)) {
		t.Fatalf("random padding value should not fall back to fixed X padding: %q", paddingValue)
	}
}

func TestFillStreamRequest_SkipsPaddingWhenConfiguredOff(t *testing.T) {
	c := Config{
		Path: "/sh?pd=off&ed=8192",
		XPaddingBytes: &RangeConfig{
			From: 24,
			To:   24,
		},
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/sh?ed=8192", nil)
	if err != nil {
		t.Fatal(err)
	}

	c.FillStreamRequest(req, "", "")

	referer := req.Header.Get("Referer")
	if referer == "" {
		t.Fatal("expected Referer to be preserved when pd=off")
	}

	refererURL, err := url.Parse(referer)
	if err != nil {
		t.Fatal(err)
	}

	if refererURL.Scheme != "https" || refererURL.Host != "example.com" {
		t.Fatalf("unexpected Referer origin: %q", referer)
	}
	if refererURL.Path != "/" {
		t.Fatalf("expected Referer path to be root only, got %q", refererURL.Path)
	}
	if refererURL.RawQuery != "" {
		t.Fatalf("expected Referer query to be empty when pd=off, got %q", refererURL.RawQuery)
	}
	if req.URL.RawQuery != "ed=8192" {
		t.Fatalf("expected request query to stay unchanged, got %q", req.URL.RawQuery)
	}
}

func TestExtractXPaddingFromRequest_DefaultModeSupportsConfiguredKeys(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		referer   string
		wantValue string
		wantKey   string
	}{
		{
			name:      "default key",
			path:      "/sh",
			referer:   "https://example.com/sh?x_padding=default123",
			wantValue: "default123",
			wantKey:   "x_padding",
		},
		{
			name:      "custom key",
			path:      "/sh?pd=custom_pad",
			referer:   "https://example.com/sh?custom_pad=custom123",
			wantValue: "custom123",
			wantKey:   "custom_pad",
		},
		{
			name:      "random key",
			path:      "/sh?pd=random",
			referer:   "https://example.com/sh?AbC123xy=random123",
			wantValue: "random123",
			wantKey:   "AbC123xy",
		},
		{
			name:      "off",
			path:      "/sh?pd=off",
			referer:   "",
			wantValue: "",
			wantKey:   "disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/sh", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Referer", tt.referer)

			c := Config{Path: tt.path}
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

func TestIsPaddingValid_AllowsMissingPaddingWhenConfiguredOff(t *testing.T) {
	c := Config{Path: "/sh?pd=off"}

	if !c.IsPaddingValid("", 100, 1000, PaddingMethodRepeatX) {
		t.Fatal("expected missing padding to be valid when pd=off")
	}
	if c.IsPaddingValid("abc", 100, 1000, PaddingMethodRepeatX) {
		t.Fatal("expected unexpected padding to be invalid when pd=off")
	}
}
