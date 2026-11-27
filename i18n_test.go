package main

import (
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
)

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
		{"zh-Hant", "ui.page.n", []any{3}, "頁面 3"},
		{"ja", "ui.page.n", []any{3}, "ページ 3"},
		{"en", "no.such.key", nil, "no.such.key"},
	}
	for _, c := range cases {
		if got := trLang(c.lang, c.key, c.args...); got != c.want {
			t.Errorf("trLang(%q, %q, %v) = %q, want %q", c.lang, c.key, c.args, got, c.want)
		}
	}
}

func TestNormalizeLang(t *testing.T) {
	cases := []struct {
		lang, want string
	}{
		{"en", "en"},
		{"en-US", "en"},
		{" EN_us ", "en"},
		{"zh", "zh"},
		{"zh-CN", "zh"},
		{"zh-SG", "zh"},
		{"zh-Hant", "zh-Hant"},
		{"zh-TW", "zh-Hant"},
		{"zh_tw", "zh-Hant"},
		{"zh-HK", "zh-Hant"},
		{"zh-MO", "zh-Hant"},
		{"ja", "ja"},
		{"ja-JP", "ja"},
		{"JA_jp", "ja"},
		{"fr", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeLang(c.lang); got != c.want {
			t.Errorf("normalizeLang(%q) = %q, want %q", c.lang, got, c.want)
		}
	}
}

func TestCatalogTranslations(t *testing.T) {
	cases := []struct {
		lang, key string
		want      string
	}{
		{"zh-TW", "ui.start", "開始任務"},
		{"zh-HK", "ui.start", "開始任務"},
		{"ja-JP", "ui.start", "タスクを開始"},
		{"zh-Hant", "ui.settings.appearance", "外觀與語言"},
		{"ja", "ui.settings.appearance", "外観と言語"},
		{"zh-Hant", "notice.settingsSaved", "設定已儲存"},
		{"ja", "notice.settingsSaved", "設定を保存しました"},
		{"zh-Hant", "err.pageCount", "頁面數量必須介於 2 到 10"},
		{"ja", "err.pageCount", "ページ数は 2 から 10 の範囲で指定してください"},
		{"zh-Hant", "syncerr.NOT_FOUND", "此變體下找不到此頁面/內容 · 可略過此步繼續，或回到變體首頁"},
		{"ja", "syncerr.NOT_FOUND", "このバリアントにこのページ/コンテンツはありません · このステップをスキップして再開するか、バリアントのホームに戻ってください"},
		{"zh-Hant", "naverr.CERT_EXPIRED", "此網站的安全性憑證已過期"},
		{"ja", "naverr.CERT_EXPIRED", "このサイトのセキュリティ証明書は期限切れです"},
		{"zh-Hant", "op.input", "在 #name 輸入「hello」"},
		{"ja", "op.input", "#name に「hello」と入力"},
	}
	for _, c := range cases {
		got := trLang(c.lang, c.key)
		if c.key == "op.input" {
			got = trLang(c.lang, c.key, "hello", "#name")
		}
		if got != c.want {
			t.Errorf("trLang(%q, %q) = %q, want %q", c.lang, c.key, got, c.want)
		}
	}
}

// TestCatalogParity keeps all four key sets identical, non-empty, and
// compatible with the same fmt placeholders.
func TestCatalogParity(t *testing.T) {
	base := catalogs["zh"]
	for _, lang := range []string{"zh", "en", "zh-Hant", "ja"} {
		catalog := catalogs[lang]
		if len(catalog) != len(base) {
			t.Errorf("%s catalog has %d keys, want %d", lang, len(catalog), len(base))
		}
		for key, want := range base {
			got, ok := catalog[key]
			if !ok {
				t.Errorf("key %q missing from %s catalog", key, lang)
				continue
			}
			if strings.TrimSpace(got) == "" {
				t.Errorf("key %q has an empty %s translation", key, lang)
			}
			if gotVerbs, wantVerbs := formatVerbs(got), formatVerbs(want); gotVerbs != wantVerbs {
				t.Errorf("key %q has %s placeholders %q, want %q", key, lang, gotVerbs, wantVerbs)
			}
		}
		for key := range catalog {
			if _, ok := base[key]; !ok {
				t.Errorf("key %q in %s catalog is missing from zh catalog", key, lang)
			}
		}
	}
}

var formatVerbPattern = regexp.MustCompile(`%(?:\[[0-9]+\])?[a-zA-Z]`)

func formatVerbs(s string) string {
	verbs := formatVerbPattern.FindAllString(s, -1)
	sort.Strings(verbs)
	return strings.Join(verbs, ",")
}

func TestCurrentLang(t *testing.T) {
	a := newApp()
	if got := a.currentLang(); got != "zh" {
		t.Fatalf("default currentLang = %q, want zh", got)
	}
	if got := a.tr("ui.start"); got != "开始任务" {
		t.Fatalf("tr default = %q, want 开始任务", got)
	}
	// The panel publishes a changed language; off-thread readers pick it up.
	a.langVal.Store("en")
	if got := a.currentLang(); got != "en" {
		t.Fatalf("published currentLang = %q, want en", got)
	}
	if got := a.tr("ui.start"); got != "Start task" {
		t.Fatalf("tr after publish = %q, want Start task", got)
	}
	// A zero-value app that has not published yet falls back to zh.
	var zero app
	if got := zero.currentLang(); got != "zh" {
		t.Fatalf("zero currentLang = %q, want zh", got)
	}
}

// TestCurrentLangConcurrent exercises the publisher/reader split that replaces
// the old unsynchronized a.lang access; run with -race to validate it.
func TestCurrentLangConcurrent(t *testing.T) {
	a := newApp()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				a.langVal.Store("en")
				a.langVal.Store("zh")
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = a.tr("ui.start")
			}
		}()
	}
	wg.Wait()
}
