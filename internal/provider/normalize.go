package provider

import (
	"net/netip"
	"regexp"
	"strings"
)

// The API stores values in a standard form (see "How values are stored" in the API docs). These
// functions compute that form, so the provider can tell when the value in a configuration and the
// stored value mean the same thing, and keep the configuration's spelling instead of showing a change.

// normalizeZone: lower case, no trailing dot.
func normalizeZone(z string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(z)), ".")
}

// normalizeName: a record name relative to the zone, lower case; "@" for the zone itself.
func normalizeName(name, zone string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	zone = normalizeZone(zone)
	if n == "" || n == "@" || n == zone || n == zone+"." {
		return "@"
	}
	n = strings.TrimSuffix(n, ".")
	return strings.TrimSuffix(n, "."+zone)
}

// normalizeAnswer: the stored form of a record's answer for its type.
func normalizeAnswer(typ, answer, zone string) string {
	a := strings.TrimSpace(answer)
	switch strings.ToUpper(typ) {
	case "A", "AAAA":
		if ip, err := netip.ParseAddr(a); err == nil {
			return ip.String()
		}
		return a
	case "CNAME", "NS", "MX", "SRV":
		t := strings.ToLower(a)
		if t == "." {
			return t
		}
		if t == "@" {
			return normalizeZone(zone)
		}
		return strings.TrimSuffix(t, ".")
	case "TXT":
		return stripQuotes(a)
	}
	return a
}

// normalizeCAA: the stored form of a CAA value for its tag.
func normalizeCAA(tag, value string) string {
	v := stripQuotes(strings.TrimSpace(value))
	if strings.ToLower(strings.TrimSpace(tag)) == "iodef" {
		return v
	}
	parts := strings.Split(v, ";")
	domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(parts[0])), ".")
	var kv []string
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if m := caaParam.FindStringSubmatch(p); m != nil {
			kv = append(kv, m[1]+"="+m[2])
		} else {
			kv = append(kv, p)
		}
	}
	if domain == "" {
		return ";"
	}
	return strings.Join(append([]string{domain}, kv...), "; ")
}

var caaParam = regexp.MustCompile(`(?i)^([a-z0-9]+)\s*=\s*(\S*)$`)

func stripQuotes(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return s[1 : len(s)-1]
	}
	return s
}
