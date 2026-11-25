package redact

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/yeegz/repropack/format"
)

func TestDefaultRules(t *testing.T) {
	p := Default()
	cases := []struct{ in, rule string }{
		{"Authorization: Bearer abcdefghijklmnop123", "bearer"},
		{"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", "jwt"},
		{"postgres://admin:hunter22@db:5432/app", "url-credentials"},
		{"GET /cb?code=abcd1234&state=x", "query-secret"},
		{"key AKIAIOSFODNN7EXAMPLE", "aws-access-key"},
		{"ghp_" + strings.Repeat("a", 36), "github-token"},
		{"xoxb-123456789012-abc", "slack-token"},
		{"AIza" + strings.Repeat("B", 35), "google-api-key"},
		{"sk_live_" + strings.Repeat("9", 24), "stripe-key"},
		{"sk-ant-" + strings.Repeat("x", 30), "llm-api-key"},
		{`{"password": "correct-horse"}`, "assignment-secret"},
		{"contact jane.doe@example.org now", "email"},
		{"card 4111 1111 1111 1111", "card-number"},
		{"open /Users/jane/Library/x.db", "home-path"},
		{"peer 8.8.8.8 timed out", "ipv4"},
		{"-----BEGIN RSA PRIVATE KEY-----\nMIIB\n-----END RSA PRIVATE KEY-----", "private-key"},
	}
	for _, c := range cases {
		fs := p.Scan("f", c.in)
		if len(fs) == 0 || fs[0].RuleID != c.rule {
			t.Errorf("%q: got %+v want %s", c.in, fs, c.rule)
			continue
		}
		out := p.String("f", c.in, nil, nil)
		if strings.Contains(out, fs[0].Value) {
			t.Errorf("%q: value survived: %q", c.in, out)
		}
		if again := p.Scan("f", out); len(again) != 0 {
			t.Errorf("%q: not idempotent, rescan %+v in %q", c.in, again, out)
		}
	}
}

func TestNoFalsePositives(t *testing.T) {
	p := Default()
	for _, s := range []string{
		"order 1234 5678 9012 3456 shipped", // fails Luhn
		"listening on 127.0.0.1:8080 and 192.168.1.4",
		"version 1.2.3",
		"the token was refreshed",
		"user clicked Save",
	} {
		if fs := p.Scan("f", s); len(fs) != 0 {
			t.Errorf("%q: unexpected %+v", s, fs)
		}
	}
}

func TestKeyRules(t *testing.T) {
	p := Default()
	c := Counts{}
	v := p.Value("attrs", map[string]any{"user_id": "42", "Session-Token": "abc", "nested": map[string]any{"cookie": "x=y"}}, c).(map[string]any)
	if v["Session-Token"] != "[REDACTED:sensitive-key]" || v["nested"].(map[string]any)["cookie"] != "[REDACTED:sensitive-key]" || v["user_id"] != "42" {
		t.Fatal(v)
	}
	if c["sensitive-key"] != 2 {
		t.Fatal(c)
	}
}

func TestKeepSelection(t *testing.T) {
	p := Default()
	in := "a@b.co and c@d.co"
	out := p.String("f", in, nil, map[int]bool{0: true})
	if out != "a@b.co and [REDACTED:email]" {
		t.Fatal(out)
	}
}

func TestRedactReportVerifies(t *testing.T) {
	var buf bytes.Buffer
	w := format.NewWriter(&buf)
	now := time.Now()
	w.WriteEvent(format.Event{Time: now, Message: "login jane@example.com", Attrs: map[string]any{"token": "s3cr3t", "path": "/Users/jane/app"}})
	w.WriteEvent(format.Event{Time: now, Kind: format.KindError, Error: &format.ErrorInfo{Message: "dial postgres://u:pw1234@h failed", Stack: []format.Frame{{File: "/home/jane/src/main.go"}}, Cause: &format.ErrorInfo{Message: "Bearer abcdefghijklmnopq"}}})
	w.AddAttachment(format.Attachment{ID: "log", MediaType: "text/plain", Role: format.RoleLog}, strings.NewReader("api_key=ABCDEFGH1234\n"))
	w.AddAttachment(format.Attachment{ID: "shot", MediaType: "image/png", Role: format.RoleScreenshot}, strings.NewReader("png"))
	w.Close(format.Manifest{Title: "crash for jane@example.com", Producer: format.Producer{Name: "t"}, Context: map[string]any{"password": "pw"}})
	r, err := format.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), format.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := ScanReport(r, Default())
	if len(before) < 7 {
		t.Fatalf("expected findings before, got %d: %+v", len(before), before)
	}
	var out bytes.Buffer
	m, err := RedactReport(r, &out, Default(), Options{ReviewedAttachments: []string{"shot"}})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Redaction.Verified || len(m.Redaction.Rules) == 0 {
		t.Fatal(m.Redaction)
	}
	raw := out.String()
	for _, secret := range []string{"jane@example.com", "s3cr3t", "pw1234", "abcdefghijklmnopq", "ABCDEFGH1234"} {
		if strings.Contains(raw, secret) {
			t.Errorf("%q survived in archive bytes", secret)
		}
	}
	r2, _ := format.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()), format.Limits{})
	if len(UnreviewedMedia(&r2.Manifest)) != 0 {
		t.Fatal("shot should be reviewed")
	}
	// Redacting twice is stable and accumulates counts.
	var out2 bytes.Buffer
	m2, err := RedactReport(r2, &out2, Default(), Options{})
	if err != nil || m2.Redaction.Rules[0].Matches != m.Redaction.Rules[0].Matches {
		t.Fatal(err, m2.Redaction)
	}
}

func FuzzRedactString(f *testing.F) {
	f.Add("Bearer abcdefghijklmnop a@b.co /Users/x 4111111111111111")
	f.Fuzz(func(t *testing.T, s string) {
		p := Default()
		out := p.String("f", s, nil, nil)
		if fs := p.Scan("f", out); len(fs) != 0 {
			// Replacement text must never itself create a new match.
			for _, x := range fs {
				if strings.Contains(x.Value, "REDACTED") {
					t.Fatalf("replacement re-matched: %q", out)
				}
			}
		}
	})
}

func BenchmarkScan1KB(b *testing.B) {
	p := Default()
	s := strings.Repeat("GET /api/items?id=42 200 12ms user=jane@example.com ", 20)
	b.SetBytes(int64(len(s)))
	for i := 0; i < b.N; i++ {
		p.Scan("f", s)
	}
}
