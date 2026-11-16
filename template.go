package main

import (
	"net/url"
	"regexp"
	"strings"
)

// placeholderRe matches a {name} token in a URL template.
var placeholderRe = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// parseVars reads "host=a, lang=en" assignments. It returns the offending
// token (empty when well-formed) so the caller can localize the error.
func parseVars(s string) (map[string]string, string) {
	vars := map[string]string{}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !ok || k == "" {
			return nil, part
		}
		vars[k] = v
	}
	return vars, ""
}

// renderTemplate substitutes {name} placeholders from vars and returns any
// placeholder names left without a value.
func renderTemplate(t string, vars map[string]string) (string, []string) {
	var missing []string
	out := placeholderRe.ReplaceAllStringFunc(t, func(m string) string {
		name := placeholderRe.FindStringSubmatch(m)[1]
		if v, ok := vars[name]; ok {
			return v
		}
		missing = append(missing, name)
		return m
	})
	return out, missing
}

// cleanTemplateBase normalizes a rendered variant base. It rejects query and
// fragment components — a path appended after them would land inside the query
// or fragment instead of the path — and trims trailing slashes so base+path
// never doubles a slash. ok is false when the base is unusable as a prefix.
func cleanTemplateBase(base string) (string, bool) {
	u, err := url.Parse(base)
	if err != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", false
	}
	return strings.TrimRight(base, "/"), true
}
