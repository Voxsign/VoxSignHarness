// Package mail is the kernel's standard EMAIL CLIENT capability: receive,
// read full bodies and attachments, understand message threads, and send
// receipts or commands over IMAP/SMTP or a provider API (such as Microsoft
// Graph).
//
// Scope: this is a CLIENT/adapter layer. It is NOT a mail transfer agent and
// does not accept, relay, queue, or store mail for a domain. A real MTA
// (Maddy, Postfix, a hosted provider) is an external system the kernel talks
// to. Keeping the kernel customer-neutral means every concrete transport
// (IMAP, Graph, a file inbox for tests) implements the same small interfaces
// and the kernel never holds provider credentials itself.
package mail

import (
	"context"
	"time"
)

// Address is a parsed mailbox with an optional display name.
type Address struct {
	Name    string
	Address string // lowercased bare address
}

// Attachment is a complete, decoded message attachment.
type Attachment struct {
	Filename string
	MIMEType string
	Content  []byte
}

// Message is a fully fetched inbound message.
type Message struct {
	// ID is the source-unique idempotency key (mailbox#UID for IMAP, the file
	// name for a file inbox); it must never collide across mailboxes/tenants.
	ID string
	// AckID is the transport-level id needed to acknowledge the message
	// (the bare IMAP UID); it falls back to ID when empty.
	AckID string

	// RFC5322 threading and correlation headers (angle brackets stripped).
	MessageID  string
	InReplyTo  string
	References []string

	From    Address
	To      []Address
	Cc      []Address
	Subject string
	Date    time.Time

	TextBody string
	HTMLBody string

	Attachments []*Attachment
}

// OutboundMessage is a message the agent wants to send. MessageID, when set, is
// the caller-chosen deterministic idempotency key (the angle-addr value without
// brackets): retries of the same logical mail carry the same id so a provider
// or downstream reconciler can collapse duplicates.
type OutboundMessage struct {
	From     string
	To       []string
	Cc       []string
	Subject  string
	TextBody string
	HTMLBody string

	Attachments []*Attachment

	MessageID  string
	InReplyTo  string
	References []string
}

// Envelope is the lightweight metadata returned by List, without bodies.
type Envelope struct {
	ID        string
	AckID     string
	MessageID string
	InReplyTo string
	From      string
	Subject   string
	Date      time.Time
	Seen      bool
}

// ListOptions bounds a mailbox listing.
type ListOptions struct {
	OnlyUnseen bool
	Since      time.Time
	Limit      int
}

// AckResult tells the transport what to do with a message after processing.
type AckResult struct {
	Seen   bool // mark read on success
	Delete bool // remove/archive when explicitly required
	Error string // when non-empty, leave the message unacknowledged for retry
}

// Lister lists message envelopes.
type Lister interface {
	List(ctx context.Context, opt ListOptions) ([]Envelope, error)
}

// Fetcher retrieves one full message (bodies and attachments) by ID.
type Fetcher interface {
	Fetch(ctx context.Context, id string) (*Message, error)
}

// Sender sends a message and returns the provider-assigned correlation id when
// the transport exposes one (Graph object id, SMTP queue id). An empty return
// is valid; reconciliation then matches via the sent folder / threading.
type Sender interface {
	Send(ctx context.Context, m *OutboundMessage) (serverMsgID string, err error)
}

// Acker acknowledges a processed inbound message.
type Acker interface {
	Ack(ctx context.Context, id string, res AckResult) error
}

// Mailbox is the full client surface a transport adapter implements.
type Mailbox interface {
	Lister
	Fetcher
	Sender
	Acker
}
