package mail

import (
	"errors"
	"strings"
)

// Outbound guard errors. These are deterministic, pre-send rejections and must
// never reach the action ledger as a performed side effect.
var (
	ErrNoRecipient       = errors.New("mail: no recipient")
	ErrBlockedRecipient  = errors.New("mail: recipient outside the allowed tenant scope")
	ErrDoNotSend         = errors.New("mail: the caller explicitly said not to send")
	ErrRiskyContent      = errors.New("mail: risky content requires explicit confirmation")
)

// SendIntent carries conversational evidence about the send, separate from the
// message itself. It lets a negation spoken by the user ("don't send",
// "不要发") veto the action even after a draft exists.
type SendIntent struct {
	DoNotSend      bool
	Confirmed      bool
	RawInstruction string
}

// GuardPolicy is a customer-neutral allow-list. Empty domain/address lists mean
// "no domain restriction configured"; once a list is present it is a hard
// tenant boundary that confirmation cannot bypass.
type GuardPolicy struct {
	AllowedDomains       []string
	AllowedAddresses     []string
	BlockedAttachmentExt []string
}

// OutboundGuard is the pre-send safety gate: recipient/tenant scope, explicit
// negation, and dangerous-attachment confirmation.
type OutboundGuard struct {
	domains     map[string]bool
	addresses   map[string]bool
	blockedExt  map[string]bool
}

// NewOutboundGuard builds a guard from a policy, applying a default dangerous
// extension list when none is supplied.
func NewOutboundGuard(p GuardPolicy) *OutboundGuard {
	g := &OutboundGuard{domains: map[string]bool{}, addresses: map[string]bool{}, blockedExt: map[string]bool{}}
	for _, d := range p.AllowedDomains {
		g.domains[strings.ToLower(strings.TrimSpace(d))] = true
	}
	for _, a := range p.AllowedAddresses {
		g.addresses[strings.ToLower(strings.TrimSpace(a))] = true
	}
	ext := p.BlockedAttachmentExt
	if len(ext) == 0 {
		ext = defaultDangerousExts
	}
	for _, e := range ext {
		g.blockedExt[strings.ToLower(strings.TrimSpace(e))] = true
	}
	return g
}

var defaultDangerousExts = []string{
	".exe", ".scr", ".bat", ".cmd", ".com", ".js", ".vbs",
	".msi", ".ps1", ".jar", ".app", ".apk", ".dll", ".sh",
}

// Check validates a message against the policy and the caller's intent.
func (g *OutboundGuard) Check(m *OutboundMessage, intent SendIntent) error {
	if intent.DoNotSend || detectDoNotSend(intent.RawInstruction) {
		return ErrDoNotSend
	}
	recipients := append(append([]string{}, m.To...), m.Cc...)
	if len(recipients) == 0 {
		return ErrNoRecipient
	}

	hasScope := len(g.domains) > 0 || len(g.addresses) > 0
	if hasScope {
		for _, rcpt := range recipients {
			addr := strings.ToLower(strings.TrimSpace(rcpt))
			if g.addresses[addr] {
				continue
			}
			if i := strings.LastIndex(addr, "@"); i >= 0 && g.domains[addr[i+1:]] {
				continue
			}
			return ErrBlockedRecipient
		}
	}

	if !intent.Confirmed {
		for _, att := range m.Attachments {
			if g.blockedExt[extOf(att.Filename)] {
				return ErrRiskyContent
			}
		}
	}
	return nil
}

func extOf(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	return strings.ToLower(name[i:])
}

// detectDoNotSend recognises explicit English and Chinese negations. It is
// intentionally conservative: only clear, imperative cancellations match, so a
// message body that merely discusses "not sending" does not veto a real send.
func detectDoNotSend(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	if l == "" {
		return false
	}
	phrases := []string{
		"don't send", "do not send", "dont send", "stop sending", "never send",
		"不要发", "别发", "先别发", "不要发送", "暂不发", "取消发送", "先不要发",
	}
	for _, p := range phrases {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}
