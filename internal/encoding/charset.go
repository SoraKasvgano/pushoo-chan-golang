package encoding

import (
	"bytes"
	"errors"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// DecodeBytes converts bytes in the given charset into UTF-8 bytes.
// If charset is empty or "utf-8", it returns the original bytes.
func DecodeBytes(b []byte, charset string) ([]byte, error) {
	cs := strings.TrimSpace(strings.ToLower(charset))
	if cs == "" || cs == "utf-8" || cs == "utf8" {
		return b, nil
	}
	enc, ok := lookupEncoding(cs)
	if !ok {
		return nil, errors.New("unsupported charset: " + charset)
	}
	r := transform.NewReader(bytes.NewReader(b), enc.NewDecoder())
	return ioReadAll(r)
}

func lookupEncoding(cs string) (encoding.Encoding, bool) {
	switch cs {
	case "gbk", "gb2312":
		return simplifiedchinese.GBK, true
	case "gb18030":
		return simplifiedchinese.GB18030, true
	case "big5":
		return traditionalchinese.Big5, true
	case "shift_jis", "shift-jis", "sjis":
		return japanese.ShiftJIS, true
	case "euc-jp":
		return japanese.EUCJP, true
	case "euc-kr":
		return korean.EUCKR, true
	case "iso-8859-1":
		return charmap.ISO8859_1, true
	case "windows-1252":
		return charmap.Windows1252, true
	default:
		return nil, false
	}
}

