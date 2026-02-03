package encoding

import (
	"bytes"
	"net/url"
	"strings"
)

// ExtractCharsetFromBytes tries to find "charset=xxx" (form) or `"charset":"xxx"` (json)
// from raw bytes by scanning ASCII sequences, so it works even when the body is not UTF-8.
func ExtractCharsetFromBytes(b []byte) string {
	// Look for "charset=" first (common in x-www-form-urlencoded).
	if cs := scanAfterASCII(b, []byte("charset="), isCharsetChar, '&'); cs != "" {
		return cs
	}
	// Look for JSON-ish `"charset": "xxx"`.
	if cs := scanJSONCharset(b); cs != "" {
		return cs
	}
	return ""
}

func scanAfterASCII(b, needle []byte, allow func(byte) bool, stop byte) string {
	idx := bytes.Index(bytes.ToLower(b), bytes.ToLower(needle))
	if idx < 0 {
		return ""
	}
	start := idx + len(needle)
	end := start
	for end < len(b) {
		c := b[end]
		if c == stop {
			break
		}
		if !allow(c) {
			break
		}
		end++
	}
	if end <= start {
		return ""
	}
	return strings.ToLower(string(b[start:end]))
}

func isCharsetChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_'
}

func scanJSONCharset(b []byte) string {
	lb := bytes.ToLower(b)
	key := []byte(`"charset"`)
	idx := bytes.Index(lb, key)
	if idx < 0 {
		return ""
	}
	// Find ':' after the key.
	colon := bytes.IndexByte(lb[idx+len(key):], ':')
	if colon < 0 {
		return ""
	}
	p := idx + len(key) + colon + 1
	for p < len(lb) && (lb[p] == ' ' || lb[p] == '\t' || lb[p] == '\n' || lb[p] == '\r') {
		p++
	}
	// Optional quote.
	if p >= len(lb) || lb[p] != '"' {
		return ""
	}
	p++
	start := p
	for p < len(lb) && isCharsetChar(lb[p]) {
		p++
	}
	if p <= start {
		return ""
	}
	// Ensure closing quote exists.
	if p >= len(lb) || lb[p] != '"' {
		return ""
	}
	return string(lb[start:p])
}

// ParseQueryWithCharset parses a raw query string where values may be percent-encoded bytes in a non-UTF8 charset.
// It is only used when the caller explicitly specifies a charset (to match the TS behavior).
func ParseQueryWithCharset(rawQuery string, charset string) (url.Values, error) {
	// Manual parser to avoid net/url forcing UTF-8.
	v := url.Values{}
	if rawQuery == "" {
		return v, nil
	}
	rawQuery = strings.TrimPrefix(rawQuery, "?")
	pairs := strings.Split(rawQuery, "&")
	for _, p := range pairs {
		if p == "" {
			continue
		}
		k, val, _ := strings.Cut(p, "=")
		keyBytes, err := url.QueryUnescape(k)
		if err != nil {
			// fallback: keep raw
			keyBytes = k
		}
		rawVal := val
		// We need percent-decoding into bytes; url.QueryUnescape gives a UTF-8 string,
		// so instead unescape to bytes via QueryUnescape on a Latin-1-ish intermediate.
		decodedBytes := percentDecodeToBytes(rawVal)
		utf8Bytes, err := DecodeBytes(decodedBytes, charset)
		if err != nil {
			utf8Bytes = decodedBytes
		}
		v.Add(keyBytes, string(utf8Bytes))
	}
	return v, nil
}

func percentDecodeToBytes(s string) []byte {
	// url.QueryUnescape returns string; we want raw bytes. Implement minimal %xx decode.
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '+' {
			out = append(out, ' ')
			continue
		}
		if c == '%' && i+2 < len(s) {
			h1, h2 := fromHex(s[i+1]), fromHex(s[i+2])
			if h1 >= 0 && h2 >= 0 {
				out = append(out, byte(h1<<4|h2))
				i += 2
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

func fromHex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return -1
	}
}

