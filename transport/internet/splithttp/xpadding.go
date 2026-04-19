package splithttp

import (
	"crypto/rand"
	"math"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http2/hpack"
)

type PaddingMethod string

const (
	PaddingMethodRepeatX      PaddingMethod = "repeat-x"
	PaddingMethodTokenish     PaddingMethod = "tokenish"
	paddingMethodRandomBase62 PaddingMethod = "random-base62"
)

const charsetBase62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const defaultRefererPaddingKey = "x_padding"
const pathPaddingKeyConfig = "pd"
const randomPaddingKeyConfigValue = "random"
const offPaddingKeyConfigValue = "off"

func generateRandomPaddingKey() string {
	b := make([]byte, 1)
	for {
		if _, err := rand.Read(b); err != nil {
			return defaultRefererPaddingKey
		}
		if b[0] < 252 {
			length := int(b[0]%63) + 2
			if key, ok := randStringFromCharset(length, charsetBase62); ok {
				return key
			}
			return defaultRefererPaddingKey
		}
	}
}

// Huffman encoding gives ~20% size reduction for base62 sequences
const avgHuffmanBytesPerCharBase62 = 0.8

const validationTolerance = 2

type XPaddingPlacement struct {
	Placement string
	Key       string
	Header    string
	RawURL    string
}

type XPaddingConfig struct {
	Length    int
	Placement XPaddingPlacement
	Method    PaddingMethod
}

func randStringFromCharset(n int, charset string) (string, bool) {
	if n <= 0 || len(charset) == 0 {
		return "", false
	}

	m := len(charset)
	limit := byte(256 - (256 % m))

	result := make([]byte, n)
	i := 0

	buf := make([]byte, 256)
	for i < n {
		if _, err := rand.Read(buf); err != nil {
			return "", false
		}
		for _, rb := range buf {
			if rb >= limit {
				continue
			}
			result[i] = charset[int(rb)%m]
			i++
			if i == n {
				break
			}
		}
	}

	return string(result), true
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func GenerateTokenishPaddingBase62(targetHuffmanBytes int) string {
	n := int(math.Ceil(float64(targetHuffmanBytes) / avgHuffmanBytesPerCharBase62))
	if n < 1 {
		n = 1
	}

	randBase62Str, ok := randStringFromCharset(n, charsetBase62)
	if !ok {
		return ""
	}

	const maxIter = 150
	adjustChar := byte('X')

	// Adjust until close enough
	for iter := 0; iter < maxIter; iter++ {
		currentLength := int(hpack.HuffmanEncodeLength(randBase62Str))
		diff := currentLength - targetHuffmanBytes

		if absInt(diff) <= validationTolerance {
			return randBase62Str
		}

		if diff < 0 {
			// Too small -> append padding char(s)
			randBase62Str += string(adjustChar)

			// Avoid a long run of identical chars
			if adjustChar == 'X' {
				adjustChar = 'Z'
			} else {
				adjustChar = 'X'
			}
		} else {
			// Too big -> remove from the end
			if len(randBase62Str) <= 1 {
				return randBase62Str
			}
			randBase62Str = randBase62Str[:len(randBase62Str)-1]
		}
	}

	return randBase62Str
}

func GeneratePadding(method PaddingMethod, length int) string {
	if length <= 0 {
		return ""
	}

	// https://www.rfc-editor.org/rfc/rfc7541.html#appendix-B
	// h2's HPACK Header Compression feature employs a huffman encoding using a static table.
	// 'X' and 'Z' are assigned an 8 bit code, so HPACK compression won't change actual padding length on the wire.
	// https://www.rfc-editor.org/rfc/rfc9204.html#section-4.1.2-2
	// h3's similar QPACK feature uses the same huffman table.

	switch method {
	case PaddingMethodRepeatX:
		return strings.Repeat("X", length)
	case paddingMethodRandomBase62:
		paddingValue, ok := randStringFromCharset(length, charsetBase62)
		if !ok {
			return strings.Repeat("X", length)
		}
		return paddingValue
	case PaddingMethodTokenish:
		paddingValue := GenerateTokenishPaddingBase62(length)
		if paddingValue == "" {
			return strings.Repeat("X", length)
		}
		return paddingValue
	default:
		return strings.Repeat("X", length)
	}
}

func ApplyPaddingToCookie(req *http.Request, name, value string) {
	if req == nil || name == "" || value == "" {
		return
	}
	req.AddCookie(&http.Cookie{
		Name:  name,
		Value: value,
		Path:  "/",
	})
}

func ApplyPaddingToResponseCookie(writer http.ResponseWriter, name, value string) {
	if name == "" || value == "" {
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name:  name,
		Value: value,
		Path:  "/",
	})
}

func ApplyPaddingToQuery(u *url.URL, key, value string) {
	if u == nil || key == "" || value == "" {
		return
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
}

func (c *Config) GetNormalizedXPaddingBytes() RangeConfig {
	if c.XPaddingBytes == nil || c.XPaddingBytes.To == 0 {
		return RangeConfig{
			From: 100,
			To:   1000,
		}
	}

	return *c.XPaddingBytes
}

func getDefaultRefererURL(rawURL string) string {
	refererURL := rawURL
	if parsedURL, err := url.Parse(rawURL); err == nil && parsedURL != nil {
		parsedURL.Path = "/"
		parsedURL.RawPath = ""
		parsedURL.RawQuery = ""
		parsedURL.Fragment = ""
		refererURL = parsedURL.String()
	}

	return refererURL
}

func defaultRefererPaddingConfig(rawURL string, length int, key string) XPaddingConfig {
	if key == randomPaddingKeyConfigValue {
		key = generateRandomPaddingKey()
	}

	return XPaddingConfig{
		Length: length,
		Placement: XPaddingPlacement{
			Placement: PlacementQueryInHeader,
			Key:       key,
			Header:    "Referer",
			RawURL:    getDefaultRefererURL(rawURL),
		},
		Method: paddingMethodRandomBase62,
	}
}

func getFirstQueryValue(values url.Values, keys ...string) (string, string) {
	for _, key := range keys {
		if value := values.Get(key); value != "" {
			return value, key
		}
	}
	return "", ""
}

func getFirstNonEmptyQueryValue(values url.Values) (string, string) {
	for key, items := range values {
		for _, value := range items {
			if value != "" {
				return value, key
			}
		}
	}

	return "", ""
}

func (c *Config) getConfiguredPaddingValue(values url.Values) (string, string) {
	paddingKey := c.GetNormalizedPathPaddingKey()
	if paddingKey == offPaddingKeyConfigValue {
		return "", offPaddingKeyConfigValue
	}
	if paddingKey == randomPaddingKeyConfigValue {
		if paddingValue, actualKey := getFirstNonEmptyQueryValue(values); actualKey != "" {
			return paddingValue, actualKey
		}
		return "", randomPaddingKeyConfigValue
	}

	return values.Get(paddingKey), paddingKey
}

func (c *Config) ApplyXPaddingToHeader(h http.Header, config XPaddingConfig) {
	if h == nil {
		return
	}

	paddingValue := GeneratePadding(config.Method, config.Length)

	switch p := config.Placement; p.Placement {
	case PlacementHeader:
		h.Set(p.Header, paddingValue)
	case PlacementQueryInHeader:
		u, err := url.Parse(p.RawURL)
		if err != nil || u == nil {
			return
		}
		u.RawQuery = appendRawQueryParameter(u.RawQuery, p.Key, paddingValue)
		h.Set(p.Header, u.String())
	}
}

func (c *Config) ApplyXPaddingToRequest(req *http.Request, config XPaddingConfig) {
	if req == nil {
		return
	}
	if req.Header == nil {
		req.Header = make(http.Header)
	}

	placement := config.Placement.Placement

	if placement == PlacementHeader || placement == PlacementQueryInHeader {
		c.ApplyXPaddingToHeader(req.Header, config)
		return
	}

	paddingValue := GeneratePadding(config.Method, config.Length)

	switch placement {
	case PlacementCookie:
		ApplyPaddingToCookie(req, config.Placement.Key, paddingValue)
	case PlacementQuery:
		ApplyPaddingToQuery(req.URL, config.Placement.Key, paddingValue)
	}
}

func (c *Config) ApplyXPaddingToResponse(writer http.ResponseWriter, config XPaddingConfig) {
	placement := config.Placement.Placement

	if placement == PlacementHeader || placement == PlacementQueryInHeader {
		c.ApplyXPaddingToHeader(writer.Header(), config)
		return
	}

	paddingValue := GeneratePadding(config.Method, config.Length)

	switch placement {
	case PlacementCookie:
		ApplyPaddingToResponseCookie(writer, config.Placement.Key, paddingValue)
	}
}

func (c *Config) ExtractXPaddingFromRequest(req *http.Request, obfsMode bool) (string, string) {
	if req == nil {
		return "", ""
	}

	if !obfsMode {
		if c.GetNormalizedPathPaddingKey() == offPaddingKeyConfigValue {
			return "", "disabled"
		}

		referrer := req.Header.Get("Referer")

		if referrer != "" {
			if referrerURL, err := url.Parse(referrer); err == nil {
				paddingValue, paddingKey := c.getConfiguredPaddingValue(referrerURL.Query())
				paddingPlacement := PlacementQueryInHeader + "=Referer, key=" + paddingKey
				return paddingValue, paddingPlacement
			}
		} else {
			paddingValue, paddingKey := c.getConfiguredPaddingValue(req.URL.Query())
			return paddingValue, PlacementQuery + ", key=" + paddingKey
		}
	}

	key := c.XPaddingKey
	header := c.XPaddingHeader

	if cookie, err := req.Cookie(key); err == nil {
		if cookie != nil && cookie.Value != "" {
			paddingValue := cookie.Value
			paddingPlacement := PlacementCookie + ", key=" + key
			return paddingValue, paddingPlacement
		}
	}

	headerValue := req.Header.Get(header)

	if headerValue != "" {
		if c.XPaddingPlacement == PlacementHeader {
			paddingPlacement := PlacementHeader + "=" + header
			return headerValue, paddingPlacement
		}

		if parsedURL, err := url.Parse(headerValue); err == nil {
			paddingPlacement := PlacementQueryInHeader + "=" + header + ", key=" + key

			return parsedURL.Query().Get(key), paddingPlacement
		}
	}

	queryValue := req.URL.Query().Get(key)

	if queryValue != "" {
		paddingPlacement := PlacementQuery + ", key=" + key
		return queryValue, paddingPlacement
	}

	return "", ""
}

func (c *Config) IsPaddingValid(paddingValue string, from, to int32, method PaddingMethod) bool {
	if !c.XPaddingObfsMode && c.GetNormalizedPathPaddingKey() == offPaddingKeyConfigValue {
		return paddingValue == ""
	}

	if paddingValue == "" {
		return false
	}
	if to <= 0 {
		r := c.GetNormalizedXPaddingBytes()
		from, to = r.From, r.To
	}

	switch method {
	case PaddingMethodRepeatX:
		n := int32(len(paddingValue))
		return n >= from && n <= to
	case PaddingMethodTokenish:
		const tolerance = int32(validationTolerance)

		n := int32(hpack.HuffmanEncodeLength(paddingValue))
		f := from - tolerance
		t := to + tolerance
		if f < 0 {
			f = 0
		}
		return n >= f && n <= t
	default:
		n := int32(len(paddingValue))
		return n >= from && n <= to
	}
}
