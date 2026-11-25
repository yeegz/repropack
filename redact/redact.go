// Package redact finds and removes sensitive values from report text.
//
// Redaction is pattern based and therefore best effort. The package is
// designed so a redaction can be re-verified: Apply records which rules
// matched and how often (never the values), and Scan run afterwards must
// return no findings for the applied rules. Images and recordings cannot be
// scanned; they require human review (see format.RedactionInfo).
package redact

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Rule matches one kind of sensitive value.
type Rule struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	// Pattern is a Go RE2 regular expression. If it has a capture group
	// named "v", only that group is replaced.
	Pattern string `json:"pattern,omitempty"`
	// Keys redacts attribute values whose key contains one of these
	// case-insensitive substrings (e.g. "password").
	Keys []string `json:"keys,omitempty"`
	// Validate optionally post-filters regex matches (e.g. Luhn).
	Validate func(string) bool `json:"-"`
	// Replacement defaults to "[REDACTED:<id>]".
	Replacement string `json:"replacement,omitempty"`
	// Hints are literals, at least one of which must occur (compared
	// case-insensitively when FoldHints is set) for the pattern to run.
	// They are a fast prefilter and never change what matches.
	Hints     []string `json:"hints,omitempty"`
	FoldHints bool     `json:"fold_hints,omitempty"`
	// Pre is an optional programmatic prefilter (built-in rules only).
	Pre func(string) bool `json:"-"`

	re *regexp.Regexp
	vi int
}

// Policy is an ordered set of rules.
type Policy struct {
	Name  string  `json:"name"`
	Rules []*Rule `json:"rules"`
}

// Finding is one match. Value is only populated by Scan for local review UIs
// and must never be written to a report.
type Finding struct {
	RuleID string `json:"rule_id"`
	Field  string `json:"field"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Value  string `json:"-"`
	// Preview is the match with most characters masked, safe to display.
	Preview string `json:"preview"`
}

// Compile prepares the policy. It must be called before use; built-in
// policies are already compiled.
func (p *Policy) Compile() error {
	seen := map[string]bool{}
	for _, r := range p.Rules {
		if r.ID == "" {
			return fmt.Errorf("redact: rule without id")
		}
		if seen[r.ID] {
			return fmt.Errorf("redact: duplicate rule %q", r.ID)
		}
		seen[r.ID] = true
		if r.Pattern == "" && len(r.Keys) == 0 {
			return fmt.Errorf("redact: rule %q has neither pattern nor keys", r.ID)
		}
		if r.Pattern != "" {
			re, err := regexp.Compile(r.Pattern)
			if err != nil {
				return fmt.Errorf("redact: rule %q: %w", r.ID, err)
			}
			r.re = re
			r.vi = re.SubexpIndex("v")
		}
		for i, k := range r.Keys {
			r.Keys[i] = strings.ToLower(k)
		}
		if r.FoldHints {
			for i, h := range r.Hints {
				r.Hints[i] = strings.ToLower(h)
			}
		}
	}
	return nil
}

func (r *Rule) replacement() string {
	if r.Replacement != "" {
		return r.Replacement
	}
	return "[REDACTED:" + r.ID + "]"
}

// LoadPolicy reads a JSON policy file. Custom rules may be appended to the
// defaults with {"extends": "default", ...}.
func LoadPolicy(path string) (*Policy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Name    string   `json:"name"`
		Extends string   `json:"extends"`
		Disable []string `json:"disable"`
		Rules   []*Rule  `json:"rules"`
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("redact: %s: %w", path, err)
	}
	p := &Policy{Name: raw.Name}
	if raw.Extends == "default" {
		d := Default()
		off := map[string]bool{}
		for _, id := range raw.Disable {
			off[id] = true
		}
		for _, r := range d.Rules {
			if !off[r.ID] {
				p.Rules = append(p.Rules, r)
			}
		}
	} else if raw.Extends != "" {
		return nil, fmt.Errorf("redact: unknown base policy %q", raw.Extends)
	}
	p.Rules = append(p.Rules, raw.Rules...)
	if p.Name == "" {
		p.Name = "custom"
	}
	return p, p.Compile()
}

// Scan returns every match in text, sorted by position, with overlapping
// matches collapsed to the earliest rule.
func (p *Policy) Scan(field, text string) []Finding {
	var out []Finding
	var lower string
	for _, r := range p.Rules {
		if r.re == nil {
			continue
		}
		if r.Pre != nil && !r.Pre(text) {
			continue
		}
		if len(r.Hints) > 0 {
			hay := text
			if r.FoldHints {
				if lower == "" {
					lower = strings.ToLower(text)
				}
				hay = lower
			}
			hit := false
			for _, h := range r.Hints {
				if strings.Contains(hay, h) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		for _, m := range r.re.FindAllStringSubmatchIndex(text, -1) {
			s, e := m[0], m[1]
			if r.vi > 0 && m[2*r.vi] >= 0 {
				s, e = m[2*r.vi], m[2*r.vi+1]
			}
			v := text[s:e]
			if v == "" || strings.HasPrefix(v, "[REDACTED:") || v == r.replacement() {
				continue
			}
			if r.Validate != nil && !r.Validate(v) {
				continue
			}
			out = append(out, Finding{RuleID: r.ID, Field: field, Start: s, End: e, Value: v, Preview: mask(v)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	// Drop overlaps.
	res := out[:0]
	end := -1
	for _, f := range out {
		if f.Start < end {
			continue
		}
		res = append(res, f)
		end = f.End
	}
	return res
}

// KeyRule returns the first rule whose Keys match an attribute key.
func (p *Policy) KeyRule(key string) *Rule {
	lk := strings.ToLower(key)
	for _, r := range p.Rules {
		for _, k := range r.Keys {
			if strings.Contains(lk, k) {
				return r
			}
		}
	}
	return nil
}

// Counts accumulates matches per rule.
type Counts map[string]int

// Add merges other into c.
func (c Counts) Add(other Counts) {
	for k, v := range other {
		c[k] += v
	}
}

// String redacts text, adding to counts. skip, when non-nil, lists finding
// indices (as returned by Scan on the same text) the user chose to keep.
func (p *Policy) String(field, text string, counts Counts, keep map[int]bool) string {
	fs := p.Scan(field, text)
	if len(fs) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	for i, f := range fs {
		if keep[i] {
			continue
		}
		b.WriteString(text[last:f.Start])
		b.WriteString(p.rule(f.RuleID).replacement())
		last = f.End
		if counts != nil {
			counts[f.RuleID]++
		}
	}
	b.WriteString(text[last:])
	return b.String()
}

func (p *Policy) rule(id string) *Rule {
	for _, r := range p.Rules {
		if r.ID == id {
			return r
		}
	}
	return &Rule{ID: id}
}

// Value redacts a JSON-like value recursively: strings are scanned, and any
// map entry whose key matches a key rule is replaced wholesale.
func (p *Policy) Value(field string, v any, counts Counts) any {
	switch t := v.(type) {
	case string:
		return p.String(field, t, counts, nil)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			if r := p.KeyRule(k); r != nil && !isEmpty(vv) {
				if s, ok := vv.(string); ok && strings.HasPrefix(s, "[REDACTED:") {
					out[k] = s
					continue
				}
				out[k] = r.replacement()
				if counts != nil {
					counts[r.ID]++
				}
				continue
			}
			out[k] = p.Value(field+"."+k, vv, counts)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, vv := range t {
			out[i] = p.Value(fmt.Sprintf("%s[%d]", field, i), vv, counts)
		}
		return out
	}
	return v
}

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	}
	return false
}

func mask(v string) string {
	r := []rune(v)
	if len(r) <= 4 {
		return strings.Repeat("•", len(r))
	}
	keep := 2
	if len(r) > 12 {
		keep = 3
	}
	return string(r[:keep]) + strings.Repeat("•", min(len(r)-keep*2, 12)) + string(r[len(r)-keep:])
}
