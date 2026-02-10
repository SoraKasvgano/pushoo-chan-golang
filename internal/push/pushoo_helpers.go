package push

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"golang.org/x/net/html"
)

var markdownConverter = goldmark.New()

var (
	urlRegexp      = regexp.MustCompile(`https?://[^\s]+`)
	ipRegexp       = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)
	emailRegexp    = regexp.MustCompile("[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*")
	multiNLRegexp  = regexp.MustCompile(`\n{3,}`)
	trimSpaceRegex = regexp.MustCompile(`[ \t]+`)
)

var blockTags = map[string]bool{
	"p":       true,
	"div":     true,
	"br":      true,
	"li":      true,
	"ul":      true,
	"ol":      true,
	"table":   true,
	"tr":      true,
	"section": true,
	"header":  true,
	"footer":  true,
	"article": true,
	"h1":      true,
	"h2":      true,
	"h3":      true,
	"h4":      true,
	"h5":      true,
	"h6":      true,
}

func markdownToHTML(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := markdownConverter.Convert([]byte(src), &buf); err != nil {
		return src
	}
	return buf.String()
}

func markdownToText(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	return htmlToText(markdownToHTML(src))
}

func htmlToText(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return strings.TrimSpace(stripTags(htmlStr))
	}
	var buf strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			buf.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && n.Data == "br" {
			buf.WriteByte('\n')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && blockTags[n.Data] {
			buf.WriteByte('\n')
		}
	}
	walk(doc)

	out := buf.String()
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\r", "\n")
	lines := strings.Split(out, "\n")
	for i := range lines {
		lines[i] = trimSpaceRegex.ReplaceAllString(strings.TrimSpace(lines[i]), " ")
	}
	out = strings.Join(lines, "\n")
	out = multiNLRegexp.ReplaceAllString(out, "\n\n")
	return strings.TrimSpace(out)
}

func stripTags(src string) string {
	var out strings.Builder
	inTag := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch c {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				out.WriteByte(c)
			}
		}
	}
	return out.String()
}

func getTitle(content string) string {
	txt := markdownToText(content)
	if txt == "" {
		return ""
	}
	if idx := strings.IndexByte(txt, '\n'); idx >= 0 {
		return txt[:idx]
	}
	return txt
}

func removeURLAndIP(content string) string {
	out := urlRegexp.ReplaceAllString(content, "")
	out = ipRegexp.ReplaceAllString(out, "")
	out = emailRegexp.ReplaceAllString(out, "")
	return out
}

func escapeTelegramMarkdown(text string) string {
	replacer := strings.NewReplacer("*", "\\*", "_", "\\_")
	return replacer.Replace(text)
}

func formatResponseDetail(body []byte) string {
	trim := bytes.TrimSpace(body)
	if len(trim) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(trim, &v); err == nil {
		if out, err := json.Marshal(v); err == nil {
			return string(out)
		}
	}
	return strings.TrimSpace(string(trim))
}
