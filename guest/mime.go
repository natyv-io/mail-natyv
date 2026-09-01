package main

import (
	"encoding/base64"
	"io"
	"mime/quotedprintable"
	"strings"
)

// decodeMimeBody extracts a readable plain-text rendering from a raw IMAP
// BODY[TEXT] fetch. Real messages are very often multipart (at minimum a
// text/plain + text/html alternative), each part its own
// Content-Type/Content-Transfer-Encoding header block followed by a blank
// line and (usually) base64 or quoted-printable encoded content --
// BODY[TEXT] returns all of that verbatim, unparsed. This is a real,
// minimal decoder for the common case: prefers a text/plain part, falls
// back to a naively-tag-stripped text/html part, and decodes whichever
// encoding that part declares. Deliberately doesn't handle nested
// multipart or real attachments -- out of scope for this demo.
func decodeMimeBody(raw string) string {
	boundary := findBoundary(raw)
	if boundary == "" {
		// Not multipart -- the raw body IS the message, at most needing no
		// decoding (7bit/8bit is the common case for a plain message).
		return raw
	}

	var plain, html string
	for _, part := range splitParts(raw, boundary) {
		headers, body := splitHeaders(part)
		ct := strings.ToLower(headers["content-type"])
		cte := strings.ToLower(headers["content-transfer-encoding"])
		decoded := decodeByEncoding(body, cte)
		switch {
		case strings.Contains(ct, "text/plain"):
			plain = decoded
		case strings.Contains(ct, "text/html"):
			html = decoded
		}
	}
	if plain != "" {
		return plain
	}
	if html != "" {
		return stripTags(html)
	}
	return raw
}

func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// findBoundary looks for a real multipart boundary-open line ("--<token>",
// not the closing "--<token>--" form) -- requires a real 8+ character
// token to avoid false-matching a plain-text email signature separator
// ("-- \nJohn"), which is always much shorter.
func findBoundary(raw string) string {
	for _, line := range splitLines(raw) {
		if strings.HasPrefix(line, "--") && !strings.HasSuffix(line, "--") {
			token := strings.TrimPrefix(line, "--")
			if len(token) >= 8 {
				return token
			}
		}
	}
	return ""
}

// splitParts splits raw into the segments between real boundary lines
// ("--<boundary>" opens the next part, "--<boundary>--" ends the whole
// multipart body). Preamble/epilogue text outside any boundary is dropped.
func splitParts(raw, boundary string) []string {
	open := "--" + boundary
	closeLine := "--" + boundary + "--"
	var parts []string
	var current []string
	inPart := false
	for _, line := range splitLines(raw) {
		switch line {
		case open:
			if inPart {
				parts = append(parts, strings.Join(current, "\r\n"))
			}
			current = nil
			inPart = true
		case closeLine:
			if inPart {
				parts = append(parts, strings.Join(current, "\r\n"))
			}
			return parts
		default:
			if inPart {
				current = append(current, line)
			}
		}
	}
	if inPart {
		parts = append(parts, strings.Join(current, "\r\n"))
	}
	return parts
}

// splitHeaders splits one part at its first blank line into its own
// mini-header block (Content-Type, Content-Transfer-Encoding, ...) and
// body, matching the same header/body shape a top-level message has.
func splitHeaders(part string) (map[string]string, string) {
	lines := splitLines(part)
	headers := map[string]string{}
	i := 0
	for ; i < len(lines); i++ {
		if lines[i] == "" {
			i++
			break
		}
		if colon := strings.IndexByte(lines[i], ':'); colon >= 0 {
			key := strings.ToLower(strings.TrimSpace(lines[i][:colon]))
			val := strings.TrimSpace(lines[i][colon+1:])
			headers[key] = val
		}
	}
	return headers, strings.Join(lines[i:], "\r\n")
}

func decodeByEncoding(body, encoding string) string {
	switch encoding {
	case "base64":
		clean := strings.Map(func(r rune) rune {
			if r == '\r' || r == '\n' {
				return -1
			}
			return r
		}, body)
		decoded, err := base64.StdEncoding.DecodeString(clean)
		if err != nil {
			return body
		}
		return string(decoded)
	case "quoted-printable":
		decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
		if err != nil {
			return body
		}
		return string(decoded)
	default:
		return body
	}
}

var htmlEntities = strings.NewReplacer(
	"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&nbsp;", " ",
)

// stripTags is a minimal, naive HTML-to-text fallback for when a message
// has no text/plain part at all -- strips tags and decodes the handful of
// entities real emails actually use, nothing more (no real HTML parsing).
func stripTags(html string) string {
	var out strings.Builder
	inTag := false
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			out.WriteRune(r)
		}
	}
	return htmlEntities.Replace(out.String())
}
