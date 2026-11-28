package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

const (
	verificationJSONName = "verification.json"
	verificationHTMLName = "report.html"
)

type verificationReportData struct {
	Lang              string
	Title             string
	Generated         string
	Status            string
	StatusKey         string
	Summary           string
	ChecksTitle       string
	PagesTitle        string
	DifferencesTitle  string
	NoDifferences     string
	Expected          string
	Actual            string
	Pass              string
	Fail              string
	ScreenshotMissing string
	KindLabel         string
	SelectorLabel     string
	OperatorLabel     string
	ResultLabel       string
	CheckLabel        string
	PageLabel         string
	Checks            []verificationCheck
	Pages             []verificationReportPage
	Differences       []verificationReportDifference
}

type verificationReportPage struct {
	verificationPageResult
	StatusLabel string
	Shot        reportShot
}

type verificationReportDifference struct {
	verificationDifference
	Values []verificationReportValue
}

type verificationReportValue struct {
	verificationValueAt
}

var verificationReportTmpl = template.Must(template.New("verification-report").Parse(`<!doctype html>
<html lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
body{font-family:system-ui,-apple-system,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif;margin:0;background:#f6f7f9;color:#1f2328;line-height:1.5}
.wrap{max-width:1180px;margin:0 auto;padding:32px 20px}
h1{font-size:22px;margin:0 0 4px}h2{font-size:16px;margin:28px 0 10px}
.meta{color:#656d76;font-size:13px;margin-bottom:20px;word-break:break-all}
.summary{display:flex;gap:10px;align-items:center;flex-wrap:wrap;background:#fff;border:1px solid #d0d7de;border-radius:10px;padding:14px 16px;margin-bottom:18px}
.badge{display:inline-block;padding:3px 10px;border-radius:12px;font-size:12px;font-weight:700}
.pass{background:#dafbe1;color:#116329}.fail{background:#ffebe9;color:#a40e26}
.card{background:#fff;border:1px solid #d0d7de;border-radius:10px;padding:16px;margin-bottom:14px}
.cardhead{display:flex;align-items:center;gap:10px;margin-bottom:8px}.name{font-weight:700;font-size:15px}.url{color:#656d76;font-size:12px;word-break:break-all;margin-bottom:12px}
table{width:100%;border-collapse:collapse;font-size:13px}th,td{text-align:left;vertical-align:top;padding:8px;border-bottom:1px solid #eaeef2;word-break:break-word}th{color:#656d76;font-size:12px;font-weight:600}
.kind{font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:12px}.muted{color:#656d76;font-size:12px}.error{color:#a40e26;font-size:12px;white-space:pre-wrap}
.shot{margin:14px 0 0}.shot img{max-width:100%;max-height:520px;border:1px solid #d0d7de;border-radius:6px;display:block}.shot figcaption{color:#656d76;font-size:12px;margin-top:4px}
@media (max-width:720px){.wrap{padding:20px 12px}table{display:block;overflow-x:auto}th,td{min-width:110px}}
@media (prefers-color-scheme:dark){body{background:#0d1117;color:#e6edf3}.card,.summary{background:#161b22;border-color:#30363d}.meta,.url,.muted,th,.shot figcaption{color:#768390}td{border-color:#21262d}.pass{background:#0f4423;color:#7ee787}.fail{background:#490202;color:#ff7b72}.shot img{border-color:#30363d}.error{color:#ff7b72}}
</style>
</head>
<body><div class="wrap">
<h1>{{.Title}}</h1>
<div class="meta">{{.Generated}}</div>
<div class="summary"><span class="badge {{if eq .StatusKey "pass"}}pass{{else}}fail{{end}}">{{.Status}}</span><span>{{.Summary}}</span></div>
<h2>{{.ChecksTitle}}</h2>
<div class="card"><table><thead><tr><th>{{.KindLabel}}</th><th>{{.SelectorLabel}}</th><th>{{.OperatorLabel}}</th><th>{{.Expected}}</th></tr></thead><tbody>
{{range .Checks}}<tr><td class="kind">{{.Kind}}</td><td>{{if .Selector}}{{.Selector}}{{else}}—{{end}}</td><td class="kind">{{.Operator}}</td><td>{{.Value}}</td></tr>{{end}}
</tbody></table></div>
<h2>{{.PagesTitle}}</h2>
{{range .Pages}}<div class="card">
<div class="cardhead"><span class="name">{{.Name}}</span><span class="badge {{if eq .Status "pass"}}pass{{else}}fail{{end}}">{{.StatusLabel}}</span></div>
<div class="url">{{.URL}}</div>
<table><thead><tr><th>{{$.KindLabel}}</th><th>{{$.Expected}}</th><th>{{$.Actual}}</th><th>{{$.ResultLabel}}</th></tr></thead><tbody>
{{range .Results}}<tr><td class="kind">{{.Kind}}{{if .Selector}} · {{.Selector}}{{end}}</td><td>{{.Expected}}</td><td>{{.Actual}}{{if .Error}}<div class="error">{{.Error}}</div>{{end}}</td><td>{{if .Pass}}<span class="badge pass">{{$.Pass}}</span>{{else}}<span class="badge fail">{{$.Fail}}</span>{{end}}</td></tr>{{end}}
</tbody></table>
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
{{if .ScreenshotError}}<div class="error">{{.ScreenshotError}}</div>{{end}}
{{if .Shot.Src}}<figure class="shot"><img src="{{.Shot.Src}}" alt="{{.Name}}" loading="lazy" decoding="async"><figcaption>{{.Name}}</figcaption></figure>{{else}}<div class="muted">{{$.ScreenshotMissing}}</div>{{end}}
</div>{{end}}
<h2>{{.DifferencesTitle}}</h2>
{{if .Differences}}<div class="card"><table><thead><tr><th>{{$.CheckLabel}}</th><th>{{$.PageLabel}}</th><th>{{$.Actual}}</th><th>{{$.ResultLabel}}</th></tr></thead><tbody>
{{range $difference := .Differences}}{{range .Values}}<tr><td class="kind">{{$difference.Kind}} #{{$difference.Check}}</td><td>{{.Page}}</td><td>{{.Actual}}</td><td>{{if .Pass}}<span class="badge pass">{{$.Pass}}</span>{{else}}<span class="badge fail">{{$.Fail}}</span>{{end}}</td></tr>{{end}}{{end}}
</tbody></table></div>{{else}}<div class="muted">{{.NoDifferences}}</div>{{end}}
</div></body></html>
`))

func verificationRoot() string { return filepath.Join(userArtifactRoot(), "verifications") }

func newVerificationDir() (string, error) {
	root := verificationRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(root, "verification_"+time.Now().Format("20060102-150405")+"-*")
}

func writeVerificationScreenshot(dir string, index int, data []byte) (string, error) {
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		return "", fmt.Errorf("invalid screenshot PNG: %w", err)
	}
	rel := filepath.Join("screenshots", fmt.Sprintf("page-%02d.png", index+1))
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func writeVerificationReport(dir string, report *verificationReport, lang string) (string, error) {
	out := filepath.Join(dir, verificationHTMLName)
	passed := 0
	for _, page := range report.Pages {
		if page.Status == "pass" {
			passed++
		}
	}
	data := verificationReportData{
		Lang:              lang,
		Title:             trLang(lang, "verify.report.title"),
		Generated:         trLang(lang, "verify.report.generated", report.StartedAt, report.TaskID, report.RunID),
		Status:            trLang(lang, "verify.report."+report.Status),
		StatusKey:         report.Status,
		Summary:           trLang(lang, "verify.report.summary", passed, len(report.Pages), len(report.Differences)),
		ChecksTitle:       trLang(lang, "verify.report.checks"),
		PagesTitle:        trLang(lang, "verify.report.pages"),
		DifferencesTitle:  trLang(lang, "verify.report.differences"),
		NoDifferences:     trLang(lang, "verify.report.noDifferences"),
		Expected:          trLang(lang, "verify.report.expected"),
		Actual:            trLang(lang, "verify.report.actual"),
		Pass:              trLang(lang, "verify.report.pass"),
		Fail:              trLang(lang, "verify.report.fail"),
		ScreenshotMissing: trLang(lang, "verify.report.screenshotMissing"),
		KindLabel:         trLang(lang, "verify.kind"),
		SelectorLabel:     trLang(lang, "verify.selector"),
		OperatorLabel:     trLang(lang, "verify.operator"),
		ResultLabel:       trLang(lang, "verify.result"),
		CheckLabel:        trLang(lang, "verify.check"),
		PageLabel:         trLang(lang, "verify.page"),
		Checks:            report.Checks,
		Differences:       make([]verificationReportDifference, 0, len(report.Differences)),
	}
	for _, page := range report.Pages {
		item := verificationReportPage{verificationPageResult: page, StatusLabel: trLang(lang, "verify.report."+page.Status)}
		if rel := cleanRecRelPath(page.Screenshot); rel != "" {
			item.Shot = reportShotFor(dir, filepath.Join(dir, filepath.FromSlash(rel)), page.Name)
		}
		data.Pages = append(data.Pages, item)
	}
	for _, difference := range report.Differences {
		item := verificationReportDifference{verificationDifference: difference, Values: make([]verificationReportValue, 0, len(difference.Values))}
		for _, value := range difference.Values {
			item.Values = append(item.Values, verificationReportValue{verificationValueAt: value})
		}
		data.Differences = append(data.Differences, item)
	}

	var html bytes.Buffer
	if err := verificationReportTmpl.Execute(&html, data); err != nil {
		return "", err
	}
	if err := writeVerificationFile(out, html.Bytes()); err != nil {
		return "", err
	}
	report.ReportPath = out
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeVerificationFile(filepath.Join(dir, verificationJSONName), append(raw, '\n')); err != nil {
		return "", err
	}
	return out, nil
}

func writeVerificationFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
