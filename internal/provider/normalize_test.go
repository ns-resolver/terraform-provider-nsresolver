package provider

import "testing"

func TestNormalizeName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"@", "@"}, {"", "@"}, {"example.com", "@"}, {"Example.COM.", "@"},
		{"www", "www"}, {"WWW", "www"}, {"www.example.com", "www"}, {"www.example.com.", "www"},
		{"_sip._tcp", "_sip._tcp"}, {"*.dev", "*.dev"}, {"apex", "apex"},
	} {
		if got := normalizeName(c.in, "example.com"); got != c.want {
			t.Errorf("normalizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeAnswer(t *testing.T) {
	for _, c := range []struct{ typ, in, want string }{
		{"A", " 192.0.2.1 ", "192.0.2.1"},
		{"AAAA", "2001:0DB8:0:0::1", "2001:db8::1"},
		{"AAAA", "::ffff:192.0.2.1", "::ffff:192.0.2.1"},
		{"AAAA", "2001:db8:0:0:1:0:0:1", "2001:db8::1:0:0:1"}, // two equal zero runs: the first is compressed
		{"CNAME", "Target.Example.NET.", "target.example.net"},
		{"CNAME", "@", "example.com"},
		{"MX", ".", "."},
		{"NS", "ns1.provider.net.", "ns1.provider.net"},
		{"TXT", `"v=spf1 -all"`, "v=spf1 -all"},
		{"TXT", `say "hi"`, `say "hi"`},
	} {
		if got := normalizeAnswer(c.typ, c.in, "example.com"); got != c.want {
			t.Errorf("normalizeAnswer(%s, %q) = %q, want %q", c.typ, c.in, got, c.want)
		}
	}
}

func TestNormalizeCAA(t *testing.T) {
	for _, c := range []struct{ tag, in, want string }{
		{"issue", "LetsEncrypt.org.", "letsencrypt.org"},
		{"issue", `"letsencrypt.org;validationmethods = dns-01"`, "letsencrypt.org; validationmethods=dns-01"},
		{"issuewild", ";", ";"},
		{"iodef", "mailto:Security@Example.com", "mailto:Security@Example.com"},
	} {
		if got := normalizeCAA(c.tag, c.in); got != c.want {
			t.Errorf("normalizeCAA(%s, %q) = %q, want %q", c.tag, c.in, got, c.want)
		}
	}
}
