package mail

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestComposeParseRoundTripMixed(t *testing.T) {
	orig := &OutboundMessage{
		From:     "martin@voxsign.ai",
		To:       []string{"qa@example.com", "ops@example.com"},
		Cc:       []string{"lead@example.com"},
		Subject:  "部署结果：成功 ✓",
		TextBody: "第一行\n第二行 ✓",
		HTMLBody: "<p>第一行<br>第二行 ✓</p>",
		Attachments: []*Attachment{
			{Filename: "清单.txt", MIMEType: "text/plain", Content: []byte("附件正文")},
			{Filename: "data.bin", MIMEType: "application/octet-stream", Content: []byte{0, 1, 2, 255, 254, 10}},
		},
		MessageID:  "thread-1@voxsign.ai",
		InReplyTo: "parent-1@voxsign.ai",
		References: []string{"root-0@voxsign.ai", "parent-1@voxsign.ai"},
	}
	raw, err := Compose(orig)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	m, err := ParseRFC822("x", raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Subject != orig.Subject {
		t.Fatalf("subject round-trip: got %q want %q", m.Subject, orig.Subject)
	}
	if m.TextBody != orig.TextBody {
		t.Fatalf("text body round-trip: got %q want %q", m.TextBody, orig.TextBody)
	}
	if m.HTMLBody != orig.HTMLBody {
		t.Fatalf("html body round-trip: got %q want %q", m.HTMLBody, orig.HTMLBody)
	}
	if len(m.To) != 2 || m.To[0].Address != "qa@example.com" || m.To[1].Address != "ops@example.com" {
		t.Fatalf("to round-trip: %+v", m.To)
	}
	if len(m.Cc) != 1 || m.Cc[0].Address != "lead@example.com" {
		t.Fatalf("cc round-trip: %+v", m.Cc)
	}
	if m.MessageID != "thread-1@voxsign.ai" || m.InReplyTo != "parent-1@voxsign.ai" {
		t.Fatalf("threading headers round-trip: %+v", m)
	}
	if len(m.References) != 2 || m.References[0] != "root-0@voxsign.ai" {
		t.Fatalf("references round-trip: %+v", m.References)
	}
	if len(m.Attachments) != 2 {
		t.Fatalf("want 2 attachments, got %d", len(m.Attachments))
	}
	if m.Attachments[0].Filename != "清单.txt" || !bytes.Equal(m.Attachments[0].Content, []byte("附件正文")) {
		t.Fatalf("text attachment round-trip: %+v", m.Attachments[0])
	}
	if m.Attachments[1].Filename != "data.bin" || !bytes.Equal(m.Attachments[1].Content, []byte{0, 1, 2, 255, 254, 10}) {
		t.Fatalf("binary attachment corrupted: %v", m.Attachments[1].Content)
	}
}

func TestComposeTextOnlyQuotedPrintable(t *testing.T) {
	orig := &OutboundMessage{
		From: "a@example.com", To: []string{"b@example.com"},
		Subject: "中文主题", TextBody: "中文正文 body",
		MessageID: "t2@example.com",
	}
	raw, err := Compose(orig)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	m, err := ParseRFC822("y", raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.TextBody != orig.TextBody || m.HTMLBody != "" {
		t.Fatalf("text-only round-trip: text=%q html=%q", m.TextBody, m.HTMLBody)
	}
}

func TestFileMailboxEndToEnd(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	mb := NewFileMailbox(root)

	// Send with a deterministic idempotency key and attachments.
	out := &OutboundMessage{
		From: "martin@voxsign.ai", To: []string{"qa@example.com"}, Cc: []string{"ops@example.com"},
		Subject: "回执", TextBody: "done",
		Attachments: []*Attachment{{Filename: "a.txt", MIMEType: "text/plain", Content: []byte("hi")}},
		MessageID:  "m-1@test",
	}
	id, err := mb.Send(ctx, out)
	if err != nil || id != "m-1@test" {
		t.Fatalf("send: id=%q err=%v", id, err)
	}
	if !mb.Contains("m-1@test", "qa@example.com") {
		t.Fatalf("strict probe must find the mail for its real recipient")
	}
	if mb.Contains("m-1@test", "other@example.com") {
		t.Fatalf("an unrelated recipient must NOT confirm the send")
	}
	// Re-sending the same logical key collapses to one file.
	if _, err := mb.Send(ctx, out); err != nil {
		t.Fatalf("idempotent resend: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "sent")); len(entries) != 1 {
		t.Fatalf("same Message-ID must yield one sent file, got %d", len(entries))
	}

	// Place an inbound message and process it.
	inRaw, err := Compose(&OutboundMessage{
		From: "ops@example.com", To: []string{"martin@voxsign.ai"},
		Subject: "新任务", TextBody: "请跑测试", MessageID: "in-1@example.com",
	})
	if err != nil {
		t.Fatalf("compose inbound: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "inbox", "123.eml"), inRaw, 0o644); err != nil {
		t.Fatalf("write inbox: %v", err)
	}
	env, err := mb.List(ctx, ListOptions{OnlyUnseen: true})
	if err != nil || len(env) != 1 || env[0].From != "ops@example.com" {
		t.Fatalf("list unseen: %+v err=%v", env, err)
	}
	got, err := mb.Fetch(ctx, "123.eml")
	if err != nil || got.TextBody != "请跑测试" || got.MessageID != "in-1@example.com" {
		t.Fatalf("fetch: %+v err=%v", got, err)
	}
	if err := mb.Ack(ctx, "123.eml", AckResult{Seen: true}); err != nil {
		t.Fatalf("ack seen: %v", err)
	}
	if env, _ := mb.List(ctx, ListOptions{OnlyUnseen: true}); len(env) != 0 {
		t.Fatalf("seen mail must leave the unseen inbox, got %d", len(env))
	}
	if got, err := mb.Fetch(ctx, "123.eml"); err != nil || got.Subject != "新任务" {
		t.Fatalf("seen mail must still be fetchable from seen/, got %+v err=%v", got, err)
	}

	// A processing failure must NOT lose the message.
	failRaw, _ := Compose(&OutboundMessage{From: "ops@example.com", To: []string{"martin@voxsign.ai"}, Subject: "fail", TextBody: "x"})
	if err := os.WriteFile(filepath.Join(root, "inbox", "456.eml"), failRaw, 0o644); err != nil {
		t.Fatalf("write 456: %v", err)
	}
	if err := mb.Ack(ctx, "456.eml", AckResult{Error: "handler crashed"}); err != nil {
		t.Fatalf("ack error: %v", err)
	}
	if env, _ := mb.List(ctx, ListOptions{}); len(env) != 1 || env[0].ID != "456.eml" {
		t.Fatalf("failed message must remain for retry, got %+v", env)
	}

	// Explicit delete removes the message.
	if err := mb.Ack(ctx, "456.eml", AckResult{Delete: true}); err != nil {
		t.Fatalf("ack delete: %v", err)
	}
	if _, err := mb.Fetch(ctx, "456.eml"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted message must be ErrNotFound, got %v", err)
	}
}
