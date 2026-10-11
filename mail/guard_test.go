package mail

import (
	"errors"
	"testing"
)

func TestGuardDoNotSend(t *testing.T) {
	g := NewOutboundGuard(GuardPolicy{AllowedDomains: []string{"example.com"}})
	m := &OutboundMessage{To: []string{"qa@example.com"}, Subject: "x", TextBody: "y"}

	if err := g.Check(m, SendIntent{DoNotSend: true}); !errors.Is(err, ErrDoNotSend) {
		t.Fatalf("explicit DoNotSend must veto, got %v", err)
	}
	if err := g.Check(m, SendIntent{RawInstruction: "请先别发这封邮件，等我确认"}); !errors.Is(err, ErrDoNotSend) {
		t.Fatalf("Chinese negation must veto, got %v", err)
	}
	if err := g.Check(m, SendIntent{RawInstruction: "don't send it yet"}); !errors.Is(err, ErrDoNotSend) {
		t.Fatalf("English negation must veto, got %v", err)
	}
	if err := g.Check(m, SendIntent{RawInstruction: "请把这封邮件发给QA"}); err != nil {
		t.Fatalf("a normal send instruction must pass, got %v", err)
	}
}

func TestGuardRecipientScope(t *testing.T) {
	g := NewOutboundGuard(GuardPolicy{
		AllowedDomains:   []string{"voxsign.ai"},
		AllowedAddresses: []string{"trusted@example.com"},
	})
	inScope := &OutboundMessage{To: []string{"martin@voxsign.ai", "trusted@example.com"}}
	if err := g.Check(inScope, SendIntent{}); err != nil {
		t.Fatalf("in-scope recipients must pass, got %v", err)
	}
	outScope := &OutboundMessage{To: []string{"martin@voxsign.ai", "someone@gmail.com"}}
	if err := g.Check(outScope, SendIntent{Confirmed: true}); !errors.Is(err, ErrBlockedRecipient) {
		t.Fatalf("out-of-scope recipient must be blocked even when confirmed, got %v", err)
	}
	empty := &OutboundMessage{}
	if err := g.Check(empty, SendIntent{}); !errors.Is(err, ErrNoRecipient) {
		t.Fatalf("no recipient must be rejected, got %v", err)
	}
}

func TestGuardDangerousAttachment(t *testing.T) {
	g := NewOutboundGuard(GuardPolicy{AllowedDomains: []string{"example.com"}})
	withExe := &OutboundMessage{
		To:          []string{"qa@example.com"},
		Attachments: []*Attachment{{Filename: "tool.exe", MIMEType: "application/octet-stream"}},
	}
	if err := g.Check(withExe, SendIntent{}); !errors.Is(err, ErrRiskyContent) {
		t.Fatalf("exe attachment must require confirmation, got %v", err)
	}
	if err := g.Check(withExe, SendIntent{Confirmed: true}); err != nil {
		t.Fatalf("confirmed exe attachment may pass, got %v", err)
	}
	withTxt := &OutboundMessage{
		To:          []string{"qa@example.com"},
		Attachments: []*Attachment{{Filename: "notes.txt", MIMEType: "text/plain"}},
	}
	if err := g.Check(withTxt, SendIntent{}); err != nil {
		t.Fatalf("plain text attachment must pass, got %v", err)
	}
}
