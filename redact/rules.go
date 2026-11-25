package redact

import "strings"

// Default returns the built-in policy. Each call returns a fresh copy.
//
// The rules target values that are commonly pasted into bug reports by
// accident: credentials, tokens, personal identifiers and home-directory
// paths that reveal account names.
func Default() *Policy {
	p := &Policy{Name: "default", Rules: []*Rule{
		{ID: "private-key", Hints: []string{"PRIVATE KEY"}, Description: "PEM private key blocks",
			Pattern: `-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`},
		{ID: "jwt", Hints: []string{"eyJ"}, Description: "JSON Web Tokens",
			Pattern: `\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`},
		{ID: "bearer", Hints: []string{"bearer", "basic", "token"}, FoldHints: true, Description: "Authorization bearer/basic credentials",
			Pattern: `(?i)\b(?:bearer|basic|token)\s+(?P<v>[A-Za-z0-9._~+/=-]{12,})`},
		{ID: "url-credentials", Hints: []string{"://"}, Description: "user:password@ in URLs",
			Pattern: `\b[a-zA-Z][a-zA-Z0-9+.-]*://[^\s/:@]+:(?P<v>[^\s/@]+)@`},
		{ID: "query-secret", Hints: []string{"?", "&"}, Description: "secret-looking URL query parameters",
			Pattern: `(?i)[?&](?:access_token|api_key|apikey|token|secret|password|sig|signature|code|key)=(?P<v>[^&\s#"']{4,})`},
		{ID: "aws-access-key", Hints: []string{"AKIA", "ASIA"}, Description: "AWS access key IDs",
			Pattern: `\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`},
		{ID: "github-token", Hints: []string{"gh", "github_pat_"}, Description: "GitHub tokens",
			Pattern: `\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{40,})\b`},
		{ID: "slack-token", Hints: []string{"xox"}, Description: "Slack tokens",
			Pattern: `\bxox[abposr]-[A-Za-z0-9-]{10,}\b`},
		{ID: "google-api-key", Hints: []string{"AIza"}, Description: "Google API keys",
			Pattern: `\bAIza[0-9A-Za-z_-]{35}\b`},
		{ID: "stripe-key", Hints: []string{"_live_", "_test_"}, Description: "Stripe secret and restricted keys",
			Pattern: `\b(?:sk|rk)_(?:live|test)_[0-9A-Za-z]{16,}\b`},
		{ID: "llm-api-key", Hints: []string{"sk-"}, Description: "AI provider API keys",
			Pattern: `\bsk-(?:ant-|proj-)?[A-Za-z0-9_-]{20,}\b`},
		{ID: "assignment-secret", Hints: []string{"pass", "pwd", "secret", "token", "key"}, FoldHints: true, Description: "password=..., secret: ... style assignments",
			Pattern: `(?i)\b(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|client[_-]?secret|auth[_-]?token)\b["']?\s*[:=]\s*["']?(?P<v>[^\s"',;}{]{4,})`},
		{ID: "email", Hints: []string{"@"}, Description: "email addresses",
			Pattern: `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`},
		{ID: "card-number", Pre: minDigits(13), Description: "payment card numbers (Luhn-checked)",
			Pattern: `\b(?:\d[ -]?){12,18}\d\b`, Validate: luhn},
		{ID: "home-path", Hints: []string{"/Users/", "/home/", "\\Users\\", ":/Users/"}, Description: "account names in home directory paths",
			Pattern: `(?:/Users/|/home/|C:\\Users\\|C:/Users/)(?P<v>[^/\\\s"':\[][^/\\\s"':]*)`, Replacement: "[user]"},
		{ID: "ipv4", Pre: minDots(3), Description: "IPv4 addresses (not loopback or private)",
			Pattern: `\b(?:\d{1,3}\.){3}\d{1,3}\b`, Validate: publicIPv4},
		{ID: "sensitive-key", Description: "attribute values whose key names a secret",
			Keys: []string{"password", "passwd", "secret", "token", "authorization", "cookie", "api_key", "apikey", "private_key", "session", "credential"}},
	}}
	if err := p.Compile(); err != nil {
		panic(err)
	}
	return p
}

func luhn(s string) bool {
	sum, n, alt := 0, 0, false
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c == ' ' || c == '-' {
			continue
		}
		d := int(c - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
		n++
	}
	return n >= 13 && n <= 19 && sum%10 == 0
}

func publicIPv4(s string) bool {
	parts := strings.Split(s, ".")
	var o [4]int
	for i, p := range parts {
		v := 0
		for _, c := range p {
			v = v*10 + int(c-'0')
		}
		if v > 255 || len(p) > 1 && p[0] == '0' {
			return false
		}
		o[i] = v
	}
	switch {
	case o[0] == 10, o[0] == 127, o[0] == 0:
		return false
	case o[0] == 172 && o[1] >= 16 && o[1] <= 31:
		return false
	case o[0] == 192 && o[1] == 168:
		return false
	case o[0] == 169 && o[1] == 254:
		return false
	case o[0] >= 224:
		return false
	}
	return true
}

func minDigits(n int) func(string) bool {
	return func(s string) bool {
		c := 0
		for i := 0; i < len(s); i++ {
			if s[i] >= '0' && s[i] <= '9' {
				if c++; c >= n {
					return true
				}
			}
		}
		return false
	}
}

func minDots(n int) func(string) bool {
	return func(s string) bool { return strings.Count(s, ".") >= n }
}
