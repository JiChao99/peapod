package main

import "testing"

func TestTrLang(t *testing.T) {
	cases := []struct {
		lang, key string
		args      []any
		want      string
	}{
		{"zh", "ui.start", nil, "开始任务"},
		{"en", "ui.start", nil, "Start task"},
		{"fr", "ui.start", nil, "开始任务"},
		{"en", "ui.page.n", []any{3}, "Page 3"},
		{"zh", "ui.page.n", []any{3}, "页面 3"},
		{"en", "no.such.key", nil, "no.such.key"},
	}
	for _, c := range cases {
		if got := trLang(c.lang, c.key, c.args...); got != c.want {
			t.Errorf("trLang(%q, %q, %v) = %q, want %q", c.lang, c.key, c.args, got, c.want)
		}
	}
}
