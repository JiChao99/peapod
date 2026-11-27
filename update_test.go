package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type updateRoundTripFunc func(*http.Request) (*http.Response, error)

func (f updateRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func updateHTTPResponse(status int, body io.Reader, contentLength int64) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:        make(http.Header),
		Body:          io.NopCloser(body),
		ContentLength: contentLength,
	}
}

type updateZeroReader struct{}

func (updateZeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

type updateErrorReader struct{ err error }

func (r updateErrorReader) Read([]byte) (int, error) { return 0, r.err }

func updateTestRelease() *releaseInfo {
	return &releaseInfo{
		Version: "0.11.0",
		Tag:     "v0.11.0",
		URL:     "https://github.com/JiChao99/peapod/releases/tag/v0.11.0",
		Assets: []releaseAsset{
			{Name: "peapod-setup-0.11.0-amd64.exe", URL: "https://github.com/JiChao99/peapod/releases/download/v0.11.0/peapod-setup-0.11.0-amd64.exe", Size: 10},
			{Name: "peapod-0.11.0-linux-amd64.tar.gz", URL: "https://github.com/JiChao99/peapod/releases/download/v0.11.0/peapod-0.11.0-linux-amd64.tar.gz", Size: 10},
			{Name: "peapod-0.11.0-linux-arm64.tar.gz", URL: "https://github.com/JiChao99/peapod/releases/download/v0.11.0/peapod-0.11.0-linux-arm64.tar.gz", Size: 10},
			{Name: "SHA256SUMS", URL: "https://github.com/JiChao99/peapod/releases/download/v0.11.0/SHA256SUMS", Size: 100},
		},
	}
}

func TestFetchLatestRelease(t *testing.T) {
	client := &http.Client{Transport: updateRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("Accept = %q", req.Header.Get("Accept"))
		}
		if !strings.HasPrefix(req.Header.Get("User-Agent"), "Peapod/") {
			t.Errorf("User-Agent = %q", req.Header.Get("User-Agent"))
		}
		body := `{
			"tag_name":"v0.11.0",
			"name":"Peapod 0.11.0",
			"html_url":"https://github.com/JiChao99/peapod/releases/tag/v0.11.0",
			"published_at":"2026-11-26T00:00:00Z",
			"prerelease":false,
			"draft":false,
			"assets":[{"name":"peapod-0.11.0-linux-amd64.tar.gz","browser_download_url":"https://github.com/JiChao99/peapod/releases/download/v0.11.0/peapod-0.11.0-linux-amd64.tar.gz","size":123}]
		}`
		return updateHTTPResponse(http.StatusOK, strings.NewReader(body), int64(len(body))), nil
	})}
	got, err := fetchLatestReleaseWithClient(context.Background(), latestReleaseURL, client)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "0.11.0" || got.URL != "https://github.com/JiChao99/peapod/releases/tag/v0.11.0" {
		t.Fatalf("release = %+v", got)
	}
	if len(got.Assets) != 1 || got.Assets[0].Name != "peapod-0.11.0-linux-amd64.tar.gz" || got.Assets[0].Size != 123 {
		t.Fatalf("assets = %+v", got.Assets)
	}
}

func TestFetchLatestReleaseRejectsInvalidMetadata(t *testing.T) {
	cases := []string{
		`{"tag_name":"latest","html_url":"https://github.com/JiChao99/peapod/releases/tag/latest"}`,
		`{"tag_name":"v0.11.0","html_url":"https://example.com/release"}`,
		`{"tag_name":"v0.11.0","html_url":"https://github.com:444/JiChao99/peapod/releases/tag/v0.11.0"}`,
		`{"tag_name":"v0.11.0","html_url":"https://github.com/JiChao99/peapod/releases/tag/v0.11.0","prerelease":true}`,
		`{"tag_name":"v0.11.0","html_url":"https://github.com/JiChao99/peapod/releases/tag/v0.11.0"} trailing`,
	}
	for _, body := range cases {
		client := &http.Client{Transport: updateRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return updateHTTPResponse(http.StatusOK, strings.NewReader(body), int64(len(body))), nil
		})}
		if _, err := fetchLatestReleaseWithClient(context.Background(), latestReleaseURL, client); err == nil {
			t.Errorf("invalid release accepted: %s", body)
		}
	}
}

func TestFetchLatestReleaseHTTPError(t *testing.T) {
	client := &http.Client{Transport: updateRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return updateHTTPResponse(http.StatusForbidden, strings.NewReader("rate limited"), 13), nil
	})}
	if _, err := fetchLatestReleaseWithClient(context.Background(), latestReleaseURL, client); err == nil {
		t.Fatal("HTTP error was accepted")
	}
}

func TestFetchLatestReleaseRejectsOversizedMetadata(t *testing.T) {
	client := &http.Client{Transport: updateRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return updateHTTPResponse(http.StatusOK, io.LimitReader(updateZeroReader{}, maxReleaseBytes+1), -1), nil
	})}
	if _, err := fetchLatestReleaseWithClient(context.Background(), latestReleaseURL, client); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized metadata error = %v", err)
	}
}

func TestNewerRelease(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.9.0", "0.10.0", true},
		{"0.10.0", "0.9.0", false},
		{"0.10.0", "0.10.0", false},
		{"0.10.0", "0.10.1", true},
		{"dev", "0.10.0", false},
	}
	for _, c := range cases {
		if got := newerRelease(c.current, c.latest); got != c.want {
			t.Errorf("newerRelease(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestUpdateSnapshotCopiesRelease(t *testing.T) {
	a := newApp()
	a.update.Latest = &releaseInfo{Version: "0.11.0", Assets: []releaseAsset{{Name: "asset"}}}
	got := a.updateSnapshot()
	got.Latest.Version = "changed"
	got.Latest.Assets[0].Name = "changed"
	if a.update.Latest.Version != "0.11.0" || a.update.Latest.Assets[0].Name != "asset" {
		t.Fatal("update snapshot aliases app state")
	}
}

func TestInstallUpdateWithoutRelease(t *testing.T) {
	a := newApp()
	a.downloadAndInstallUpdate()
	state := a.updateSnapshot()
	if state.Err == "" || state.Phase != "" {
		t.Fatalf("state = %+v", state)
	}
}

func TestSelectUpdateAssets(t *testing.T) {
	cases := []struct {
		name, goos, goarch, want string
		unsupported              bool
		mutate                   func(*releaseInfo)
	}{
		{name: "windows amd64", goos: "windows", goarch: "amd64", want: "peapod-setup-0.11.0-amd64.exe"},
		{name: "linux amd64", goos: "linux", goarch: "amd64", want: "peapod-0.11.0-linux-amd64.tar.gz"},
		{name: "linux arm64", goos: "linux", goarch: "arm64", want: "peapod-0.11.0-linux-arm64.tar.gz"},
		{name: "macOS fallback", goos: "darwin", goarch: "arm64", unsupported: true},
		{name: "Windows arm64 unsupported", goos: "windows", goarch: "arm64", unsupported: true},
		{name: "Linux 386 unsupported", goos: "linux", goarch: "386", unsupported: true},
		{name: "missing asset", goos: "linux", goarch: "amd64", mutate: func(r *releaseInfo) { r.Assets = []releaseAsset{r.Assets[0], r.Assets[3]} }},
		{name: "duplicate selected asset", goos: "linux", goarch: "amd64", mutate: func(r *releaseInfo) { r.Assets = append(r.Assets, r.Assets[1]) }},
		{name: "duplicate checksums", goos: "linux", goarch: "amd64", mutate: func(r *releaseInfo) { r.Assets = append(r.Assets, r.Assets[3]) }},
		{name: "untrusted asset host", goos: "linux", goarch: "amd64", mutate: func(r *releaseInfo) { r.Assets[1].URL = "https://example.com/update.tar.gz" }},
		{name: "invalid version", goos: "linux", goarch: "amd64", mutate: func(r *releaseInfo) { r.Version = "../0.11.0" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := updateTestRelease()
			if tc.mutate != nil {
				tc.mutate(info)
			}
			selected, err := selectUpdateAssets(info, tc.goos, tc.goarch)
			if tc.unsupported {
				if !errors.Is(err, ErrUpdateUnsupported) {
					t.Fatalf("error = %v, want ErrUpdateUnsupported", err)
				}
				return
			}
			if tc.mutate != nil {
				if err == nil {
					t.Fatal("invalid release assets accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if selected.Asset.Name != tc.want || selected.Checksum.Name != "SHA256SUMS" {
				t.Fatalf("selected = %+v", selected)
			}
		})
	}
}

func TestValidateUpdateURL(t *testing.T) {
	valid := []string{
		"https://api.github.com/repos/JiChao99/peapod/releases/latest",
		"https://github.com/releases/download/v0.11.0/SHA256SUMS",
		"https://objects.githubusercontent.com/asset",
		"https://release-assets.githubusercontent.com/asset",
		"https://api.github.com:443/asset",
	}
	for _, rawURL := range valid {
		if err := validateUpdateURL(rawURL); err != nil {
			t.Errorf("validateUpdateURL(%q): %v", rawURL, err)
		}
	}
	invalid := []string{
		"http://api.github.com/asset",
		"https://example.com/asset",
		"https://api.github.com:444/asset",
		"https://user:pass@api.github.com/asset",
		"https://api.github.com.evil.example/asset",
		"file:///C:/asset",
	}
	for _, rawURL := range invalid {
		if err := validateUpdateURL(rawURL); err == nil {
			t.Errorf("validateUpdateURL(%q) accepted", rawURL)
		}
	}
}

func TestDownloadRejectsInsecureInitialURLBeforeTransport(t *testing.T) {
	client := &http.Client{Transport: updateRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("insecure request reached the transport")
		return nil, nil
	})}
	err := downloadBounded(context.Background(), client, "http://api.github.com/asset", filepath.Join(t.TempDir(), "asset"), 10, nil)
	if err == nil {
		t.Fatal("insecure initial URL accepted")
	}
}

func TestDownloadChecksEveryRedirect(t *testing.T) {
	cases := []struct {
		name, location string
		allowed        bool
	}{
		{name: "HTTP downgrade", location: "http://api.github.com/asset"},
		{name: "untrusted host", location: "https://example.com/asset"},
		{name: "nonstandard port", location: "https://api.github.com:444/asset"},
		{name: "credentials", location: "https://user@api.github.com/asset"},
		{name: "allowed explicit port", location: "https://api.github.com:443/asset", allowed: true},
		{name: "allowed asset host", location: "https://release-assets.githubusercontent.com/asset", allowed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			client := newSecureUpdateHTTPClient(time.Second, updateRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if requests == 1 {
					resp := updateHTTPResponse(http.StatusFound, strings.NewReader(""), 0)
					resp.Header.Set("Location", tc.location)
					return resp, nil
				}
				if !tc.allowed {
					t.Errorf("disallowed redirect reached transport: %s", req.URL)
				}
				return updateHTTPResponse(http.StatusOK, strings.NewReader("asset"), 5), nil
			}))
			destination := filepath.Join(t.TempDir(), "asset")
			err := downloadBounded(context.Background(), client, "https://api.github.com/start", destination, 10, nil)
			if tc.allowed {
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(destination)
				if err != nil || string(data) != "asset" {
					t.Fatalf("data = %q, err = %v", data, err)
				}
			} else if err == nil {
				t.Fatal("disallowed redirect accepted")
			}
		})
	}
}

func TestDownloadBoundedSizeLimits(t *testing.T) {
	cases := []struct {
		name          string
		limit         int64
		contentLength int64
		body          io.Reader
	}{
		{name: "checksum declared size", limit: maxChecksumBytes, contentLength: maxChecksumBytes + 1, body: updateErrorReader{errors.New("must not read")}},
		{name: "asset declared size", limit: maxUpdateAssetBytes, contentLength: maxUpdateAssetBytes + 1, body: updateErrorReader{errors.New("must not read")}},
		{name: "unknown length", limit: 1024, contentLength: -1, body: io.LimitReader(updateZeroReader{}, 1025)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: updateRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return updateHTTPResponse(http.StatusOK, tc.body, tc.contentLength), nil
			})}
			destination := filepath.Join(t.TempDir(), "download")
			err := downloadBounded(context.Background(), client, "https://api.github.com/asset", destination, tc.limit, nil)
			if err == nil || !strings.Contains(err.Error(), "exceed") {
				t.Fatalf("error = %v", err)
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatalf("partial download was not removed: %v", err)
			}
		})
	}
}

func TestParseSHA256Sums(t *testing.T) {
	const name = "peapod-0.11.0-linux-amd64.tar.gz"
	digest := sha256.Sum256([]byte("update"))
	line := fmt.Sprintf("%x  %s\n", digest, name)
	got, err := parseSHA256Sums([]byte(line), name)
	if err != nil || got != digest {
		t.Fatalf("digest = %x, err = %v", got, err)
	}
	binary, err := parseSHA256Sums([]byte(fmt.Sprintf("%X *%s\r\n", digest, name)), name)
	if err != nil || binary != digest {
		t.Fatalf("binary digest = %x, err = %v", binary, err)
	}
	cases := []string{
		"",
		"not-a-checksum",
		line + line,
		fmt.Sprintf("%s  %s\n", strings.Repeat("z", 64), name),
	}
	for _, data := range cases {
		if _, err := parseSHA256Sums([]byte(data), name); err == nil {
			t.Errorf("invalid checksum manifest accepted: %q", data)
		}
	}
}

func TestDownloadAndVerifySHA256MismatchCleansTemporaryDirectory(t *testing.T) {
	info := updateTestRelease()
	selected, err := selectUpdateAssets(info, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	good := sha256.Sum256([]byte("good update"))
	sums := []byte(fmt.Sprintf("%x  %s\n", good, selected.Asset.Name))
	client := &http.Client{Transport: updateRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case selected.Checksum.URL:
			return updateHTTPResponse(http.StatusOK, bytes.NewReader(sums), int64(len(sums))), nil
		case selected.Asset.URL:
			return updateHTTPResponse(http.StatusOK, strings.NewReader("bad update"), 10), nil
		default:
			t.Fatalf("unexpected request %s", req.URL)
			return nil, nil
		}
	})}
	workDir := filepath.Join(t.TempDir(), "update")
	if err := os.Mkdir(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	verified, err := downloadAndVerifySelectedUpdate(context.Background(), selected, workDir, client, nil)
	if err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") || verified != nil {
		t.Fatalf("verified = %+v, err = %v", verified, err)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("temporary directory was not cleaned: %v", err)
	}
}

func TestDownloadAndVerifyUpdateSuccess(t *testing.T) {
	info := updateTestRelease()
	selected, err := selectUpdateAssets(info, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	asset := []byte("verified update")
	digest := sha256.Sum256(asset)
	sums := []byte(fmt.Sprintf("%x  %s\n", digest, selected.Asset.Name))
	selected.Asset.Size = int64(len(asset))
	client := &http.Client{Transport: updateRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case selected.Checksum.URL:
			return updateHTTPResponse(http.StatusOK, bytes.NewReader(sums), int64(len(sums))), nil
		case selected.Asset.URL:
			return updateHTTPResponse(http.StatusOK, bytes.NewReader(asset), int64(len(asset))), nil
		default:
			t.Fatalf("unexpected request %s", req.URL)
			return nil, nil
		}
	})}
	workDir := filepath.Join(t.TempDir(), "update")
	if err := os.Mkdir(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	var phases []string
	verified, err := downloadAndVerifySelectedUpdate(context.Background(), selected, workDir, client, func(phase string, progress float64) {
		phases = append(phases, phase)
		if progress < 0 || progress > 100 {
			t.Errorf("progress = %f", progress)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workDir)
	if verified.SHA256 != digest || verified.Asset.Name != selected.Asset.Name {
		t.Fatalf("verified = %+v", verified)
	}
	if len(phases) < 2 || phases[0] != "downloading" || phases[len(phases)-1] != "verifying" {
		t.Fatalf("phases = %v", phases)
	}
}

type updateTarEntry struct {
	name     string
	typeFlag byte
	mode     int64
	body     string
	linkname string
}

func makeUpdateTarGz(t *testing.T, entries ...updateTarEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Typeflag: entry.typeFlag, Mode: entry.mode, Linkname: entry.linkname}
		if entry.typeFlag == tar.TypeReg || entry.typeFlag == tar.TypeRegA {
			header.Size = int64(len(entry.body))
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := io.WriteString(tw, entry.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestExtractUpdateTarGzRejectsUnsafeEntries(t *testing.T) {
	cases := []struct {
		name    string
		entries []updateTarEntry
	}{
		{name: "parent traversal", entries: []updateTarEntry{{name: "../escape", typeFlag: tar.TypeReg, mode: 0o600, body: "bad"}}},
		{name: "nested traversal", entries: []updateTarEntry{{name: "safe/../../escape", typeFlag: tar.TypeReg, mode: 0o600, body: "bad"}}},
		{name: "absolute", entries: []updateTarEntry{{name: "/absolute", typeFlag: tar.TypeReg, mode: 0o600, body: "bad"}}},
		{name: "Windows drive", entries: []updateTarEntry{{name: "C:/absolute", typeFlag: tar.TypeReg, mode: 0o600, body: "bad"}}},
		{name: "symlink", entries: []updateTarEntry{{name: "link", typeFlag: tar.TypeSymlink, mode: 0o777, linkname: "/tmp"}}},
		{name: "hardlink", entries: []updateTarEntry{{name: "hard", typeFlag: tar.TypeLink, mode: 0o600, linkname: "../escape"}}},
		{name: "fifo", entries: []updateTarEntry{{name: "fifo", typeFlag: tar.TypeFifo, mode: 0o600}}},
		{name: "device", entries: []updateTarEntry{{name: "device", typeFlag: tar.TypeChar, mode: 0o600}}},
		{name: "duplicate", entries: []updateTarEntry{{name: "same", typeFlag: tar.TypeReg, mode: 0o600, body: "one"}, {name: "same", typeFlag: tar.TypeReg, mode: 0o600, body: "two"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			destination := filepath.Join(root, "staging")
			if err := os.Mkdir(destination, 0o700); err != nil {
				t.Fatal(err)
			}
			archive := makeUpdateTarGz(t, tc.entries...)
			err := extractUpdateTarGz(bytes.NewReader(archive), destination, tarExtractionLimits{MaxBytes: 1024, MaxEntries: 100})
			if err == nil {
				t.Fatal("unsafe archive accepted")
			}
			if _, err := os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
				t.Fatalf("archive escaped staging: %v", err)
			}
		})
	}
}

func TestExtractUpdateTarGzLimits(t *testing.T) {
	archive := makeUpdateTarGz(t,
		updateTarEntry{name: "one", typeFlag: tar.TypeReg, mode: 0o600, body: "123456"},
		updateTarEntry{name: "two", typeFlag: tar.TypeReg, mode: 0o600, body: "789"},
	)
	cases := []tarExtractionLimits{
		{MaxBytes: 5, MaxEntries: 10},
		{MaxBytes: 100, MaxEntries: 1},
	}
	for _, limits := range cases {
		destination := t.TempDir()
		if err := extractUpdateTarGz(bytes.NewReader(archive), destination, limits); err == nil {
			t.Fatalf("limits %+v not enforced", limits)
		}
	}
}

func TestExtractUpdateTarGzCountsMetadataAndTrailingData(t *testing.T) {
	t.Run("PAX headers", func(t *testing.T) {
		archive := makeUpdateTarGz(t, updateTarEntry{
			name: strings.Repeat("x", 300) + ".txt", typeFlag: tar.TypeReg, mode: 0o600, body: "x",
		})
		if err := extractUpdateTarGz(bytes.NewReader(archive), t.TempDir(), tarExtractionLimits{MaxBytes: 1024, MaxEntries: 10}); err == nil {
			t.Fatal("archive metadata bypassed the decompressed-byte limit")
		}
	})
	t.Run("trailing data", func(t *testing.T) {
		var buffer bytes.Buffer
		gz := gzip.NewWriter(&buffer)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(&tar.Header{Name: "peapod", Typeflag: tar.TypeReg, Mode: 0o755, Size: 3}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, "new"); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := gz.Write(make([]byte, 5000)); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		if err := extractUpdateTarGz(bytes.NewReader(buffer.Bytes()), t.TempDir(), tarExtractionLimits{MaxBytes: 4096, MaxEntries: 10}); err == nil {
			t.Fatal("trailing decompressed data bypassed the byte limit")
		}
	})
}

func TestExtractUpdateTarGzHonorsCancellation(t *testing.T) {
	archive := makeUpdateTarGz(t, updateTarEntry{name: "peapod", typeFlag: tar.TypeReg, mode: 0o755, body: "new"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := extractUpdateTarGzContext(ctx, bytes.NewReader(archive), t.TempDir(), tarExtractionLimits{MaxBytes: 4096, MaxEntries: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestExtractUpdateTarGzAndValidateLinuxPackage(t *testing.T) {
	destination := t.TempDir()
	archive := makeUpdateTarGz(t,
		updateTarEntry{name: "bin/", typeFlag: tar.TypeDir, mode: 0o755},
		updateTarEntry{name: "peapod", typeFlag: tar.TypeReg, mode: 0o755, body: "new executable"},
		updateTarEntry{name: "data/file.txt", typeFlag: tar.TypeReg, mode: 0o644, body: "data"},
	)
	if err := extractUpdateTarGz(bytes.NewReader(archive), destination, tarExtractionLimits{MaxBytes: 4096, MaxEntries: 10}); err != nil {
		t.Fatal(err)
	}
	if err := validateExtractedLinuxPackage(destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "data", "file.txt"))
	if err != nil || string(data) != "data" {
		t.Fatalf("data = %q, err = %v", data, err)
	}
}

func writeUpdateTestApp(t *testing.T, directory, executable string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "peapod"), []byte(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "obsolete.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInstallLinuxArchiveStagesAndReplaces(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "peapod.app")
	writeUpdateTestApp(t, target, "old executable")
	archive := filepath.Join(parent, "update.tar.gz")
	if err := os.WriteFile(archive, makeUpdateTarGz(t, updateTarEntry{name: "peapod", typeFlag: tar.TypeReg, mode: 0o755, body: "new executable"}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installLinuxArchive(archive, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "peapod"))
	if err != nil || string(data) != "new executable" {
		t.Fatalf("executable = %q, err = %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(target, "obsolete.txt")); !os.IsNotExist(err) {
		t.Fatalf("obsolete file survived replacement: %v", err)
	}
	leftovers, err := filepath.Glob(filepath.Join(parent, ".peapod.app.*-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary directories = %v, err = %v", leftovers, err)
	}
}

func TestInstallLinuxArchiveRollsBackFailedRename(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "peapod.app")
	writeUpdateTestApp(t, target, "old executable")
	archive := filepath.Join(parent, "update.tar.gz")
	if err := os.WriteFile(archive, makeUpdateTarGz(t, updateTarEntry{name: "peapod", typeFlag: tar.TypeReg, mode: 0o755, body: "new executable"}), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	rename := func(source, destination string) error {
		calls++
		if calls == 2 { // target -> backup succeeded; staging -> target fails.
			return errors.New("injected rename failure")
		}
		return os.Rename(source, destination)
	}
	if err := installLinuxArchiveWithRenamer(archive, target, rename); err == nil {
		t.Fatal("rename failure accepted")
	}
	data, err := os.ReadFile(filepath.Join(target, "peapod"))
	if err != nil || string(data) != "old executable" {
		t.Fatalf("rollback executable = %q, err = %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(target, "obsolete.txt")); err != nil {
		t.Fatalf("rollback lost old files: %v", err)
	}
}

func TestInstallLinuxArchiveFailureLeavesTargetUntouched(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "peapod.app")
	writeUpdateTestApp(t, target, "old executable")
	archive := filepath.Join(parent, "update.tar.gz")
	if err := os.WriteFile(archive, makeUpdateTarGz(t, updateTarEntry{name: "../escape", typeFlag: tar.TypeReg, mode: 0o755, body: "bad"}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installLinuxArchive(archive, target); err == nil {
		t.Fatal("unsafe archive installed")
	}
	data, err := os.ReadFile(filepath.Join(target, "peapod"))
	if err != nil || string(data) != "old executable" {
		t.Fatalf("executable = %q, err = %v", data, err)
	}
}

func TestValidateLinuxUpdateTargetAt(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".local", "peapod.app")
	writeUpdateTestApp(t, target, "old")
	if err := validateLinuxUpdateTargetAt(target, home); err != nil {
		t.Fatal(err)
	}
	if err := validateLinuxUpdateTargetAt(home, home); err == nil {
		t.Fatal("home directory itself accepted as installation target")
	}
	outside := filepath.Join(t.TempDir(), "peapod.app")
	writeUpdateTestApp(t, outside, "old")
	if err := validateLinuxUpdateTargetAt(outside, home); err == nil {
		t.Fatal("installation outside the user directory accepted")
	}
}

func TestWindowsInstallerCommandLinePreservesSpaces(t *testing.T) {
	installer := `C:\Program Files\Peapod\setup.exe`
	installDir := `C:\Users\A Person\Programs\Peapod App`
	got := windowsInstallerCommandLine(installer, installDir)
	want := `"C:\Program Files\Peapod\setup.exe" /S /D=C:\Users\A Person\Programs\Peapod App`
	if got != want {
		t.Fatalf("command line = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, " /D="+installDir) {
		t.Fatal("NSIS /D is not the final unquoted argument")
	}
}

func TestQuoteWindowsCommandLineArg(t *testing.T) {
	cases := map[string]string{
		`plain`:      `plain`,
		`two words`:  `"two words"`,
		`trailing \`: `"trailing \\"`,
		`a"b`:        `"a\"b"`,
		`dir\end \`:  `"dir\end \\"`,
	}
	for input, want := range cases {
		if got := quoteWindowsCommandLineArg(input); got != want {
			t.Errorf("quoteWindowsCommandLineArg(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWindowsHelperStatusRequiresCompletion(t *testing.T) {
	dir := t.TempDir()
	config := windowsUpdateHelperConfig{
		ReadyFile: filepath.Join(dir, "ready"),
		ErrorFile: filepath.Join(dir, "error.txt"),
	}
	for _, state := range []string{"", windowsHelperReady} {
		if state == "" {
			_ = os.Remove(config.ReadyFile)
		} else if err := os.WriteFile(config.ReadyFile, []byte(state), 0o600); err != nil {
			t.Fatal(err)
		}
		done, err := windowsHelperStatus(config)
		if done || err != nil {
			t.Fatalf("state %q: done = %v, err = %v", state, done, err)
		}
	}
	if err := os.WriteFile(config.ReadyFile, []byte(windowsHelperComplete), 0o600); err != nil {
		t.Fatal(err)
	}
	if done, err := windowsHelperStatus(config); !done || err != nil {
		t.Fatalf("complete: done = %v, err = %v", done, err)
	}
	if err := os.WriteFile(config.ReadyFile, []byte(windowsHelperFailed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.ErrorFile, []byte("installer failed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if done, err := windowsHelperStatus(config); !done || err == nil || !strings.Contains(err.Error(), "installer failed") {
		t.Fatalf("failed: done = %v, err = %v", done, err)
	}
}

func TestValidateWindowsHelperConfigAllowsCallingParentPID(t *testing.T) {
	parent := t.TempDir()
	installDir := filepath.Join(parent, "Peapod App")
	workDir := filepath.Join(parent, "update")
	backupRoot := filepath.Join(parent, ".peapod.backup")
	for _, dir := range []string{installDir, workDir, backupRoot} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	installDir, err := filepath.EvalSymlinks(installDir)
	if err != nil {
		t.Fatal(err)
	}
	helperPath := filepath.Join(workDir, "helper.exe")
	installer := filepath.Join(workDir, "setup.exe")
	for _, file := range []string{helperPath, installer} {
		if err := os.WriteFile(file, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := windowsUpdateHelperConfig{
		ParentPID: os.Getpid(), InstallDir: installDir, Installer: installer,
		Executable: filepath.Join(installDir, "peapod.exe"), BackupDir: backupRoot,
		WorkDir: workDir, HelperPath: helperPath, ConfigPath: filepath.Join(workDir, "helper.json"),
		ReadyFile: filepath.Join(workDir, "ready"), ErrorFile: filepath.Join(workDir, "error.txt"),
		LogPath: filepath.Join(workDir, "helper.log"), WorkingDir: parent,
	}
	if err := validateWindowsHelperConfig(config); err != nil {
		t.Fatalf("calling process PID rejected: %v", err)
	}
	config.ParentPID = 0
	if err := validateWindowsHelperConfig(config); err == nil {
		t.Fatal("invalid parent PID accepted")
	}
}

func TestReplaceWindowsInstallRollsBack(t *testing.T) {
	parent := t.TempDir()
	installDir := filepath.Join(parent, "Peapod App")
	writeUpdateTestApp(t, installDir, "old")
	// replaceWindowsInstall looks for the Windows executable name.
	if err := os.Rename(filepath.Join(installDir, "peapod"), filepath.Join(installDir, "peapod.exe")); err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(parent, "setup.exe")
	if err := os.WriteFile(installer, []byte("setup"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupRoot := filepath.Join(parent, "backup")
	if err := os.Mkdir(backupRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("installer failed")
	err := replaceWindowsInstall(installDir, installer, backupRoot, func(gotInstaller, commandLine string) error {
		if gotInstaller != installer || commandLine != windowsInstallerCommandLine(installer, installDir) {
			t.Fatalf("installer = %q, command line = %q", gotInstaller, commandLine)
		}
		if err := os.MkdirAll(installDir, 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(installDir, "partial"), []byte("partial"), 0o600); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(installDir, "peapod.exe"))
	if err != nil || string(data) != "old" {
		t.Fatalf("rollback executable = %q, err = %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "partial")); !os.IsNotExist(err) {
		t.Fatalf("partial installation survived rollback: %v", err)
	}
	if _, err := os.Stat(backupRoot); !os.IsNotExist(err) {
		t.Fatalf("backup not cleaned after rollback: %v", err)
	}
}

func TestReplaceWindowsInstallSuccessRetainsBackup(t *testing.T) {
	parent := t.TempDir()
	installDir := filepath.Join(parent, "Peapod App")
	writeUpdateTestApp(t, installDir, "old")
	if err := os.Rename(filepath.Join(installDir, "peapod"), filepath.Join(installDir, "peapod.exe")); err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(parent, "setup.exe")
	if err := os.WriteFile(installer, []byte("setup"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupRoot := filepath.Join(parent, "backup")
	if err := os.Mkdir(backupRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	err := replaceWindowsInstall(installDir, installer, backupRoot, func(_, _ string) error {
		if err := os.MkdirAll(installDir, 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(installDir, "peapod.exe"), []byte("new"), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(installDir, "peapod.exe"))
	if err != nil || string(data) != "new" {
		t.Fatalf("executable = %q, err = %v", data, err)
	}
	previous := filepath.Join(backupRoot, "previous", "peapod.exe")
	if data, err := os.ReadFile(previous); err != nil || string(data) != "old" {
		t.Fatalf("previous executable = %q, err = %v", data, err)
	}
}

func TestUpdateWaitProcessHelper(t *testing.T) {
	if os.Getenv("PEAPOD_UPDATE_WAIT_CHILD") == "1" {
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestUpdateWaitProcessHelper$")
	cmd.Env = append(os.Environ(), "PEAPOD_UPDATE_WAIT_CHILD=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := waitForProcessHandle(ctx, cmd.Process); err != nil {
		_ = cmd.Process.Kill()
		t.Fatal(err)
	}
}

func TestInstallVerifiedUpdateCancellationCleansWorkDir(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "update")
	if err := os.Mkdir(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "asset"), []byte("verified"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := installVerifiedUpdate(ctx, &verifiedUpdate{WorkDir: workDir}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("verified update directory was not cleaned: %v", err)
	}
}

func TestRunUpdateHelperRejectsWrongPlatform(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper runs on Windows")
	}
	handled, code := runUpdateHelper([]string{updateHelperFlag, "config.json"})
	if !handled || code == 0 {
		t.Fatalf("handled = %v, code = %d", handled, code)
	}
}
