package mail

import (
	"context"
	"errors"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNotFound indicates a message id is absent from the mailbox.
var ErrNotFound = errors.New("mail: message not found")

// FileInbox is a zero-dependency reference inbound mailbox: unseen messages are
// *.eml files in its root directory and Ack(Seen) moves them into ./seen. It
// powers demos, unit tests and replaying sanitized recorded scenarios without
// touching a real IMAP server.
type FileInbox struct {
	mu   sync.Mutex
	dir  string
	seen string
}

// NewFileInbox creates a file inbox rooted at dir.
func NewFileInbox(dir string) *FileInbox {
	return &FileInbox{dir: dir, seen: filepath.Join(dir, "seen")}
}

// List returns the envelopes of unseen .eml files in the inbox root.
func (f *FileInbox) List(_ context.Context, opt ListOptions) ([]Envelope, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Envelope
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".eml") {
			continue
		}
		info, _ := e.Info()
		if !opt.Since.IsZero() && info != nil && info.ModTime().Before(opt.Since) {
			continue
		}
		m, err := f.parseFileLocked(filepath.Join(f.dir, e.Name()), e.Name())
		if err != nil {
			continue
		}
		out = append(out, Envelope{
			ID: e.Name(), AckID: e.Name(), MessageID: m.MessageID,
			InReplyTo: m.InReplyTo, From: m.From.Address, Subject: m.Subject,
			Date: m.Date, Seen: false,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if opt.Limit > 0 && len(out) > opt.Limit {
		out = out[:opt.Limit]
	}
	return out, nil
}

// Fetch retrieves one full message from the inbox or, once acknowledged, seen/.
func (f *FileInbox) Fetch(_ context.Context, id string) (*Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fileExists(filepath.Join(f.dir, id)) {
		return f.parseFileLocked(filepath.Join(f.dir, id), id)
	}
	if fileExists(filepath.Join(f.seen, id)) {
		return f.parseFileLocked(filepath.Join(f.seen, id), id)
	}
	return nil, ErrNotFound
}

// Ack marks a message seen (moves to ./seen), deletes it, or leaves it for
// retry when res.Error is non-empty.
func (f *FileInbox) Ack(_ context.Context, id string, res AckResult) error {
	if res.Error != "" {
		return nil // processing failed: keep the message for a later poll
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	src := filepath.Join(f.dir, id)
	if res.Delete {
		if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if res.Seen {
		if err := os.MkdirAll(f.seen, 0o755); err != nil {
			return err
		}
		if err := os.Rename(src, filepath.Join(f.seen, id)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (f *FileInbox) parseFileLocked(path, id string) (*Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseRFC822(id, data)
}

// FileSentBox is a reference Sender that writes composed messages as .eml files
// in a directory. It also backs a strict reconciliation probe (Contains), so
// the guaranteed-send flow can be demonstrated end to end with no SMTP server.
type FileSentBox struct {
	mu  sync.Mutex
	dir string
}

// NewFileSentBox creates a sent-mail directory sender.
func NewFileSentBox(dir string) *FileSentBox {
	return &FileSentBox{dir: dir}
}

// Send composes and persists one message, keyed by its Message-ID so a retried
// send of the same logical mail overwrites the same file rather than adding a
// duplicate. It returns the correlation id.
func (s *FileSentBox) Send(_ context.Context, m *OutboundMessage) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", err
	}
	mm := *m
	id := mm.MessageID
	if id == "" {
		id = newMsgID()
		mm.MessageID = id
	}
	raw, err := Compose(&mm)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(s.dir, safeName(id)+".eml"), raw, 0o644); err != nil {
		return "", err
	}
	return id, nil
}

// Contains is a strict probe: the message with msgID exists AND was addressed
// to recipient (To or Cc). An unrelated sent message never matches.
func (s *FileSentBox) Contains(msgID, recipient string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(s.dir, safeName(msgID)+".eml"))
	if err != nil {
		return false
	}
	mm, err := ParseRFC822(msgID, data)
	if err != nil {
		return false
	}
	want := strings.ToLower(strings.TrimSpace(recipient))
	for _, a := range append(append([]Address{}, mm.To...), mm.Cc...) {
		if a.Address == want {
			return true
		}
	}
	return false
}

// FileMailbox combines a file inbox and sent box to satisfy Mailbox.
type FileMailbox struct {
	*FileInbox
	*FileSentBox
}

// NewFileMailbox creates inbox/ and sent/ under root.
func NewFileMailbox(root string) *FileMailbox {
	in := NewFileInbox(filepath.Join(root, "inbox"))
	sb := NewFileSentBox(filepath.Join(root, "sent"))
	_ = os.MkdirAll(in.dir, 0o755)
	_ = os.MkdirAll(sb.dir, 0o755)
	return &FileMailbox{
		FileInbox:  in,
		FileSentBox: sb,
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func newMsgID() string {
	var b [12]byte
	if _, err := crand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d@vhs-filemail.local", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:]) + "@vhs-filemail.local"
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			strings.ContainsRune("._@-", r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "msg"
	}
	return b.String()
}
