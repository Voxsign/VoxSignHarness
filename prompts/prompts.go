// Package prompts implements a section-based system-prompt builder distilled
// from DeepSeek Harness (packages/core/system-prompt): the system prompt is
// assembled from ordered sections (identity → persona → policy → context →
// output format), {{variable}} references are interpolated, and empty sections
// are dropped at render time. Dynamic sections evaluate per turn so runtime
// state (task slot, history, checkpoint) enters the prompt without editing the
// constant text.
package prompts

import (
	"sort"
	"strings"
)

// Section is one contributed part of the system prompt.
type Section struct {
	// Name must be unique within a Builder; a duplicate Add shadows nothing
	// but renders both (caller responsibility).
	Name string
	// Order controls ascending concatenation; negative orders are allowed so
	// the identity can lead (mirroring DeepSeek's HARNESS_IDENTITY = -1000).
	Order int
	// Text is the static section body. Ignored when Dynamic is set.
	Text string
	// Dynamic evaluates the section at render time; an empty result drops
	// the section. When nil, Text is used.
	Dynamic func() string
}

// Builder assembles ordered sections into one rendered system prompt.
type Builder struct {
	sections []Section
}

// New returns an empty Builder.
func New() *Builder { return &Builder{} }

// Add registers a static section.
func (b *Builder) Add(name string, order int, text string) *Builder {
	b.sections = append(b.sections, Section{Name: name, Order: order, Text: text})
	return b
}

// AddDynamic registers a per-turn evaluated section.
func (b *Builder) AddDynamic(name string, order int, fn func() string) *Builder {
	b.sections = append(b.sections, Section{Name: name, Order: order, Dynamic: fn})
	return b
}

// Render joins non-empty sections in ascending order with a blank line
// between them (DeepSeek renderPrompt: filter empty, join "\n\n").
func (b *Builder) Render() string {
	sort.SliceStable(b.sections, func(i, j int) bool {
		if b.sections[i].Order != b.sections[j].Order {
			return b.sections[i].Order < b.sections[j].Order
		}
		return b.sections[i].Name < b.sections[j].Name
	})
	var parts []string
	for _, s := range b.sections {
		text := s.Text
		if s.Dynamic != nil {
			text = s.Dynamic()
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n\n")
}
