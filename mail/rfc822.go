package mail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"strings"
)

const (
	defaultMaxBodyBytes  = 10 << 20
	defaultMaxAttachment = 25 << 20
)

type headerGetter interface {
	Get(string) string
}

// ParseRFC822 parses raw RFC5322 bytes into a Message, recursively walking
// nested multipart trees (mixed/alternative/related) to collect both text and
// HTML bodies plus complete, transfer-decoded attachments.
func ParseRFC822(id string, raw []byte) (*Message, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	h := msg.Header
	m := &Message{ID: id, AckID: id}
	m.MessageID = sanitizeAngle(h.Get("Message-ID"))
	m.InReplyTo = sanitizeAngle(h.Get("In-Reply-To"))
	m.References = parseReferences(h.Get("References"), h.Get("In-Reply-To"))
	m.Subject = decodeHeaderWord(h.Get("Subject"))
	m.From = parseAddr(h.Get("From"))
	m.To = parseAddrList(h.Get("To"))
	m.Cc = parseAddrList(h.Get("Cc"))
	if d, perr := netmail.ParseDate(h.Get("Date")); perr == nil {
		m.Date = d
	}
	mediaType, params, _ := mime.ParseMediaType(h.Get("Content-Type"))
	if mediaType == "" {
		mediaType = "text/plain"
	}
	walkPart(mediaType, params, h, msg.Body, defaultMaxBodyBytes, defaultMaxAttachment, m)
	return m, nil
}

// walkPart handles one MIME entity, recursing into multipart containers.
func walkPart(mediaType string, params map[string]string, h headerGetter, body io.Reader, maxBody, maxAtt int64, m *Message) {
	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(body, params["boundary"])
		if mr == nil {
			return
		}
		for {
			part, err := mr.NextPart()
			if err != nil {
				return
			}
			mt, pr, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
			if mt == "" {
				mt = "text/plain"
			}
			walkPart(mt, pr, part.Header, part, maxBody, maxAtt, m)
		}
	}
	name := partName(h)
	disposition := strings.ToLower(firstDisp(h))

	switch mediaType {
	case "text/plain":
		if name != "" && disposition == "attachment" {
			addAttachment(mediaType, name, h, body, maxAtt, m)
		} else if m.TextBody == "" {
			m.TextBody = decodePart(h.Get("Content-Transfer-Encoding"), body, maxBody)
		}
	case "text/html":
		if name != "" && disposition == "attachment" {
			addAttachment(mediaType, name, h, body, maxAtt, m)
		} else if m.HTMLBody == "" {
			m.HTMLBody = decodePart(h.Get("Content-Transfer-Encoding"), body, maxBody)
		}
	default:
		if name == "" {
			name = "attachment.bin"
		}
		addAttachment(mediaType, name, h, body, maxAtt, m)
	}
}

func addAttachment(mediaType, name string, h headerGetter, body io.Reader, maxAtt int64, m *Message) {
	data, err := io.ReadAll(io.LimitReader(body, maxAtt+1))
	if err != nil || int64(len(data)) > maxAtt {
		return
	}
	data = decodeTransfer(h.Get("Content-Transfer-Encoding"), data)
	m.Attachments = append(m.Attachments, &Attachment{
		Filename: name,
		MIMEType: mediaType,
		Content:  data,
	})
}

func decodePart(cte string, body io.Reader, max int64) string {
	raw, err := io.ReadAll(io.LimitReader(body, max+1))
	if err != nil {
		return ""
	}
	s := strings.TrimRight(string(decodeTransfer(cte, raw)), "\r\n")
	// Normalize RFC CRLF line endings to LF for callers (LLM prompts, diffs).
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// decodeTransfer reverses a MIME Content-Transfer-Encoding. On any decode error
// it falls back to the raw bytes so a message is never silently truncated.
func decodeTransfer(cte string, raw []byte) []byte {
	var r io.Reader = bytes.NewReader(raw)
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "base64":
		r = base64.NewDecoder(base64.StdEncoding, r)
	case "quoted-printable":
		r = quotedprintable.NewReader(r)
	default:
		return raw
	}
	out, err := io.ReadAll(io.LimitReader(r, int64(len(raw))*2+1024))
	if err != nil {
		return raw
	}
	return out
}

func partName(h headerGetter) string {
	if cd := h.Get("Content-Disposition"); cd != "" {
		if _, p, err := mime.ParseMediaType(cd); err == nil {
			if f := p["filename"]; f != "" {
				return decodeHeaderWord(f)
			}
		}
	}
	if ct := h.Get("Content-Type"); ct != "" {
		if _, p, err := mime.ParseMediaType(ct); err == nil {
			if n := p["name"]; n != "" {
				return decodeHeaderWord(n)
			}
		}
	}
	return ""
}

func firstDisp(h headerGetter) string {
	if cd := h.Get("Content-Disposition"); cd != "" {
		if d, _, err := mime.ParseMediaType(cd); err == nil {
			return d
		}
	}
	return ""
}

func decodeHeaderWord(s string) string {
	d := &mime.WordDecoder{}
	if decoded, err := d.DecodeHeader(s); err == nil {
		return decoded
	}
	return s
}

func parseAddr(s string) Address {
	s = strings.TrimSpace(s)
	if s == "" {
		return Address{}
	}
	if e, err := netmail.ParseAddress(s); err == nil {
		return Address{Name: e.Name, Address: strings.ToLower(e.Address)}
	}
	if i := strings.Index(s, "<"); i >= 0 {
		if j := strings.Index(s[i:], ">"); j >= 0 {
			return Address{Address: strings.ToLower(strings.TrimSpace(s[i+1 : i+j]))}
		}
	}
	return Address{Address: strings.ToLower(s)}
}

func parseAddrList(s string) []Address {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	list, err := netmail.ParseAddressList(s)
	if err != nil {
		// Tolerate malformed lists: split on commas and take what we can.
		var out []Address
		for _, part := range strings.Split(s, ",") {
			if a := parseAddr(part); a.Address != "" {
				out = append(out, a)
			}
		}
		return out
	}
	out := make([]Address, 0, len(list))
	for _, e := range list {
		out = append(out, Address{Name: e.Name, Address: strings.ToLower(e.Address)})
	}
	return out
}

func parseReferences(refs, inReplyTo string) []string {
	var out []string
	for _, tok := range strings.Fields(refs) {
		if v := sanitizeAngle(tok); v != "" {
			out = append(out, v)
		}
	}
	if v := sanitizeAngle(inReplyTo); v != "" {
		found := false
		for _, x := range out {
			if x == v {
				found = true
				break
			}
		}
		if !found {
			out = append(out, v)
		}
	}
	return out
}

func sanitizeAngle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return strings.TrimSpace(s)
}
