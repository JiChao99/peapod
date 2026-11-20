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

// cleanTemplateBase normalizes a rendered variant base. Query values are kept
// as per-page variants; fragments and a bare '?' are rejected. It trims
// trailing slashes so appending a path never doubles a slash. The origin is
// normalized like plain mode (originOf), while the template's path prefix and
// query are preserved.
func cleanTemplateBase(base string) (string, bool) {
	u, err := url.Parse(base)
	if err != nil || u.ForceQuery || u.Fragment != "" || strings.Contains(base, "#") {
		return "", false
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	clean := originOf(base) + path
	if u.RawQuery != "" {
		clean += "?" + u.RawQuery
	}
	return clean, true
}

// appendTemplatePath appends a page's optional path to the rendered template
// base. Query values from the page path supplement or override template values.
func appendTemplatePath(base, path string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	suffix, err := url.Parse(path)
	if err != nil {
		return "", err
	}

	escapedPath := strings.TrimRight(u.EscapedPath(), "/") + suffix.EscapedPath()
	decodedPath, err := url.PathUnescape(escapedPath)
	if err != nil {
		return "", err
	}
	u.Path = decodedPath
	u.RawPath = escapedPath

	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", err
	}
	pathQuery, err := url.ParseQuery(suffix.RawQuery)
	if err != nil {
		return "", err
	}
	for key, values := range pathQuery {
		query[key] = values
	}
	u.RawQuery = query.Encode()
	u.ForceQuery = u.RawQuery == "" && suffix.ForceQuery
	u.Fragment = suffix.Fragment
	u.RawFragment = suffix.RawFragment
	return u.String(), nil
}
