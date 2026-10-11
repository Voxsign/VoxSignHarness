package mail

import (
	"bytes"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strings"
	"time"
)

// Compose serializes an OutboundMessage into RFC5322 bytes. It chooses:
//
//	text/plain only, text/html only, multipart/alternative (both), or
//	multipart/mixed (alternative/single body plus attachments).
//
// Text parts are quoted-printable encoded and attachments base64 encoded so
// non-ASCII content and binary files survive SMTP and Graph transports. The
// same bytes are used by every Sender implementation.
func Compose(m *OutboundMessage) ([]byte, error) {
	body, contentType, cte, err := buildBody(m)
	if err != nil {
		return nil, err
	}

	var h bytes.Buffer
	if m.From != "" {
		fmt.Fprintf(&h, "From: %s\r\n", m.From)
	}
	if len(m.To) > 0 {
		fmt.Fprintf(&h, "To: %s\r\n", strings.Join(m.To, ", "))
	}
	if len(m.Cc) > 0 {
		fmt.Fprintf(&h, "Cc: %s\r\n", strings.Join(m.Cc, ", "))
	}
	fmt.Fprintf(&h, "Subject: %s\r\n", encodeHeader(m.Subject))
	if m.MessageID != "" {
		fmt.Fprintf(&h, "Message-ID: <%s>\r\n", m.MessageID)
	}
	if m.InReplyTo != "" {
		fmt.Fprintf(&h, "In-Reply-To: <%s>\r\n", m.InReplyTo)
	}
	if len(m.References) > 0 {
		refs := make([]string, 0, len(m.References))
		for _, r := range m.References {
			refs = append(refs, "<"+r+">")
		}
		fmt.Fprintf(&h, "References: %s\r\n", strings.Join(refs, " "))
	}
	fmt.Fprintf(&h, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&h, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&h, "Content-Type: %s\r\n", contentType)
	if cte != "" {
		fmt.Fprintf(&h, "Content-Transfer-Encoding: %s\r\n", cte)
	}
	h.WriteString("\r\n")

	var out bytes.Buffer
	out.Write(h.Bytes())
	out.Write(body)
	return out.Bytes(), nil
}

func buildBody(m *OutboundMessage) (body []byte, contentType, cte string, err error) {
	hasText := m.TextBody != ""
	hasHTML := m.HTMLBody != ""
	hasAttach := len(m.Attachments) > 0

	switch {
	case hasAttach:
		var buf bytes.Buffer
		boundary := randomBoundary()
		mw := multipart.NewWriter(&buf)
		if err := mw.SetBoundary(boundary); err != nil {
			return nil, "", "", err
		}
		if hasText && hasHTML {
			if err := writeAlternative(mw, m.TextBody, m.HTMLBody); err != nil {
				return nil, "", "", err
			}
		} else if hasHTML {
			if err := writeTextPart(mw, "text/html", m.HTMLBody); err != nil {
				return nil, "", "", err
			}
		} else {
			text := m.TextBody
			if err := writeTextPart(mw, "text/plain", text); err != nil {
				return nil, "", "", err
			}
		}
		for _, att := range m.Attachments {
			if err := writeAttachment(mw, att); err != nil {
				return nil, "", "", err
			}
		}
		if err := mw.Close(); err != nil {
			return nil, "", "", err
		}
		return buf.Bytes(), "multipart/mixed; boundary=" + boundary, "", nil

	case hasText && hasHTML:
		var buf bytes.Buffer
		boundary := randomBoundary()
		mw := multipart.NewWriter(&buf)
		if err := mw.SetBoundary(boundary); err != nil {
			return nil, "", "", err
		}
		if err := writeTextPart(mw, "text/plain", m.TextBody); err != nil {
			return nil, "", "", err
		}
		if err := writeTextPart(mw, "text/html", m.HTMLBody); err != nil {
			return nil, "", "", err
		}
		if err := mw.Close(); err != nil {
			return nil, "", "", err
		}
		return buf.Bytes(), "multipart/alternative; boundary=" + boundary, "", nil

	case hasHTML:
		var buf bytes.Buffer
		qw := quotedprintable.NewWriter(&buf)
		if _, err := qw.Write([]byte(m.HTMLBody)); err != nil {
			return nil, "", "", err
		}
		if err := qw.Close(); err != nil {
			return nil, "", "", err
		}
		return buf.Bytes(), "text/html; charset=utf-8", "quoted-printable", nil

	default:
		var buf bytes.Buffer
		qw := quotedprintable.NewWriter(&buf)
		if _, err := qw.Write([]byte(m.TextBody)); err != nil {
			return nil, "", "", err
		}
		if err := qw.Close(); err != nil {
			return nil, "", "", err
		}
		return buf.Bytes(), "text/plain; charset=utf-8", "quoted-printable", nil
	}
}

func writeAlternative(outer *multipart.Writer, text, html string) error {
	boundary := randomBoundary()
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", "multipart/alternative; boundary="+boundary)
	w, err := outer.CreatePart(h)
	if err != nil {
		return err
	}
	inner := multipart.NewWriter(w)
	if err := inner.SetBoundary(boundary); err != nil {
		return err
	}
	if err := writeTextPart(inner, "text/plain", text); err != nil {
		return err
	}
	if err := writeTextPart(inner, "text/html", html); err != nil {
		return err
	}
	return inner.Close()
}

func writeTextPart(mw *multipart.Writer, mediaType, body string) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", mediaType+"; charset=utf-8")
	h.Set("Content-Transfer-Encoding", "quoted-printable")
	w, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	qw := quotedprintable.NewWriter(w)
	if _, err := qw.Write([]byte(body)); err != nil {
		return err
	}
	return qw.Close()
}

func writeAttachment(mw *multipart.Writer, att *Attachment) error {
	h := textproto.MIMEHeader{}
	mediaType := att.MIMEType
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	h.Set("Content-Type", mime.FormatMediaType(mediaType, map[string]string{"name": att.Filename}))
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": att.Filename}))
	h.Set("Content-Transfer-Encoding", "base64")
	w, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(att.Content)
	for len(encoded) > 0 {
		n := 76
		if n > len(encoded) {
			n = len(encoded)
		}
		if _, err := io.WriteString(w, encoded[:n]+"\r\n"); err != nil {
			return err
		}
		encoded = encoded[n:]
	}
	return nil
}

func encodeHeader(s string) string {
	// QEncoding leaves ASCII intact and encodes non-ASCII header words.
	return mime.QEncoding.Encode("utf-8", s)
}

func randomBoundary() string {
	var b [12]byte
	if _, err := io.ReadFull(crand.Reader, b[:]); err != nil {
		return "vhs-boundary-" + time.Now().Format("20060102150405.000000")
	}
	return "vhs-boundary-" + hex.EncodeToString(b[:])
}
