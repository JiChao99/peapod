package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"golang.org/x/mod/semver"
)

const (
	maxChecksumBytes          = 64 << 10
	maxUpdateAssetBytes       = 64 << 20
	maxExtractedUpdateBytes   = 256 << 20
	maxExtractedUpdateEntries = 10_000
	updateHTTPHeaderTimeout   = 30 * time.Second
	updateDownloadTimeout     = 10 * time.Minute
	updateHelperFlag          = "--peapod-update-helper"
	updateHelperParentWait    = 60 * time.Second
	updateHelperStartupWait   = 10 * time.Second
	updateInstallerTimeout    = 5 * time.Minute

	windowsHelperReady    = "ready"
	windowsHelperComplete = "complete"
	windowsHelperFailed   = "failed"
)

// ErrUpdateUnsupported tells the UI to keep the release-page fallback.
var ErrUpdateUnsupported = errors.New("automatic updates are not supported on this platform")

var updateDownloadHosts = map[string]bool{
	"api.github.com":                       true,
	"github.com":                           true,
	"objects.githubusercontent.com":        true,
	"release-assets.githubusercontent.com": true,
}

type selectedUpdateAssets struct {
	Asset    releaseAsset
	Checksum releaseAsset
}

type verifiedUpdate struct {
	WorkDir   string
	AssetPath string
	Asset     releaseAsset
	SHA256    [sha256.Size]byte
}

func validateUpdateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid update URL: %w", err)
	}
	if u.Scheme != "https" || u.Opaque != "" || u.User != nil {
		return fmt.Errorf("refusing update URL %q: HTTPS without credentials is required", rawURL)
	}
	if port := u.Port(); port != "" && port != "443" {
		return fmt.Errorf("refusing update URL %q: only HTTPS port 443 is allowed", rawURL)
	}
	if !updateDownloadHosts[strings.ToLower(u.Hostname())] {
		return fmt.Errorf("refusing update URL %q: host is not allowed", rawURL)
	}
	return nil
}

func releaseHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func newSecureUpdateHTTPClient(timeout time.Duration, transport http.RoundTripper) *http.Client {
	if transport == nil {
		if base, ok := http.DefaultTransport.(*http.Transport); ok {
			cloned := base.Clone()
			cloned.ResponseHeaderTimeout = updateHTTPHeaderTimeout
			transport = cloned
		} else {
			transport = http.DefaultTransport
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many update redirects")
			}
			if err := validateUpdateURL(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}

func readBoundedReader(r io.Reader, limit int64) ([]byte, error) {
	if r == nil || limit < 0 {
		return nil, errors.New("invalid bounded reader")
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("update response exceeds %d bytes", limit)
	}
	return data, nil
}

type downloadProgressReader struct {
	r    io.Reader
	done int64
	fn   func(int64)
}

func (r *downloadProgressReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.done += int64(n)
	if n > 0 && r.fn != nil {
		r.fn(r.done)
	}
	return n, err
}

func downloadBounded(ctx context.Context, client *http.Client, rawURL, destination string, limit int64, progress func(int64)) error {
	if limit < 0 {
		return errors.New("invalid download size limit")
	}
	if err := validateUpdateURL(rawURL); err != nil {
		return err
	}
	if client == nil {
		client = newSecureUpdateHTTPClient(updateDownloadTimeout, nil)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "Peapod/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("update download returned %s", resp.Status)
	}
	if resp.ContentLength > limit {
		return fmt.Errorf("update download declares %d bytes, exceeding %d", resp.ContentLength, limit)
	}
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	created := true
	finish := func(err error) error {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil && created {
			_ = os.Remove(destination)
		}
		return err
	}
	reader := &downloadProgressReader{r: io.LimitReader(resp.Body, limit+1), fn: progress}
	n, copyErr := io.Copy(f, reader)
	if copyErr == nil && n > limit {
		copyErr = fmt.Errorf("update download exceeds %d bytes", limit)
	}
	if copyErr == nil {
		copyErr = f.Sync()
	}
	return finish(copyErr)
}

func expectedUpdateAssetName(info *releaseInfo, goos, goarch string) (string, error) {
	if info == nil {
		return "", errors.New("release metadata is unavailable")
	}
	version := strings.TrimPrefix(info.Version, "v")
	if version == "" || !semverValidUpdateVersion(version) {
		return "", fmt.Errorf("invalid release version %q", info.Version)
	}
	switch goos {
	case "windows":
		if goarch != "amd64" {
			return "", fmt.Errorf("%w: Windows installers are available only for amd64, not %s", ErrUpdateUnsupported, goarch)
		}
		return "peapod-setup-" + version + "-amd64.exe", nil
	case "linux":
		if goarch != "amd64" && goarch != "arm64" {
			return "", fmt.Errorf("%w: Linux updates support amd64 and arm64, not %s", ErrUpdateUnsupported, goarch)
		}
		return "peapod-" + version + "-linux-" + goarch + ".tar.gz", nil
	default:
		return "", fmt.Errorf("%w: %s/%s; use the release page %s", ErrUpdateUnsupported, goos, goarch, info.URL)
	}
}

func semverValidUpdateVersion(version string) bool {
	return semver.IsValid("v" + version)
}

func selectUpdateAssets(info *releaseInfo, goos, goarch string) (*selectedUpdateAssets, error) {
	name, err := expectedUpdateAssetName(info, goos, goarch)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(info.Assets))
	for _, asset := range info.Assets {
		if asset.Name != "" {
			counts[asset.Name]++
		}
	}
	for assetName, count := range counts {
		if count > 1 {
			return nil, fmt.Errorf("release contains duplicate asset %q", assetName)
		}
	}
	if counts["SHA256SUMS"] != 1 {
		return nil, errors.New("release must contain exactly one SHA256SUMS asset")
	}
	if counts[name] != 1 {
		return nil, fmt.Errorf("release has no unique asset %q for %s/%s", name, goos, goarch)
	}
	selected := &selectedUpdateAssets{}
	for _, asset := range info.Assets {
		switch asset.Name {
		case name:
			selected.Asset = asset
		case "SHA256SUMS":
			selected.Checksum = asset
		}
	}
	for _, asset := range []releaseAsset{selected.Asset, selected.Checksum} {
		if err := validateUpdateURL(asset.URL); err != nil {
			return nil, err
		}
		if asset.Size < 0 {
			return nil, fmt.Errorf("asset %q has a negative size", asset.Name)
		}
	}
	if selected.Asset.Size > maxUpdateAssetBytes {
		return nil, fmt.Errorf("asset %q exceeds the %d-byte limit", name, maxUpdateAssetBytes)
	}
	if selected.Checksum.Size > maxChecksumBytes {
		return nil, fmt.Errorf("SHA256SUMS exceeds the %d-byte limit", maxChecksumBytes)
	}
	return selected, nil
}

func parseSHA256Sums(data []byte, assetName string) ([sha256.Size]byte, error) {
	var expected [sha256.Size]byte
	found := 0
	for lineNumber, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		if len(line) < 67 || line[64] != ' ' || (line[65] != ' ' && line[65] != '*') {
			return expected, fmt.Errorf("invalid SHA256SUMS line %d", lineNumber+1)
		}
		digest, err := hex.DecodeString(line[:64])
		if err != nil || len(digest) != sha256.Size {
			return expected, fmt.Errorf("invalid SHA256SUMS line %d", lineNumber+1)
		}
		name := line[66:]
		if name == "" || strings.HasPrefix(name, `\`) || strings.ContainsAny(name, `/\`) {
			return expected, fmt.Errorf("invalid SHA256SUMS filename on line %d", lineNumber+1)
		}
		if name != assetName {
			continue
		}
		found++
		copy(expected[:], digest)
	}
	switch found {
	case 0:
		return expected, fmt.Errorf("SHA256SUMS has no entry for %q", assetName)
	case 1:
		return expected, nil
	default:
		return expected, fmt.Errorf("SHA256SUMS has multiple entries for %q", assetName)
	}
}

func verifyUpdateSHA256(filePath string, expected [sha256.Size]byte) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	sum := sha256.New()
	_, copyErr := io.Copy(sum, f)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	actual := sum.Sum(nil)
	if !bytes.Equal(actual, expected[:]) {
		return fmt.Errorf("SHA256 mismatch: got %s, expected %s", hex.EncodeToString(actual), hex.EncodeToString(expected[:]))
	}
	return nil
}

func downloadAndVerifyUpdate(ctx context.Context, info *releaseInfo, goos, goarch string, client *http.Client, progress func(string, float64)) (*verifiedUpdate, error) {
	selected, err := selectUpdateAssets(info, goos, goarch)
	if err != nil {
		return nil, err
	}
	workDir, err := os.MkdirTemp("", "peapod-update-")
	if err != nil {
		return nil, err
	}
	return downloadAndVerifySelectedUpdate(ctx, selected, workDir, client, progress)
}

func downloadAndVerifySelectedUpdate(ctx context.Context, selected *selectedUpdateAssets, workDir string, client *http.Client, progress func(string, float64)) (verified *verifiedUpdate, err error) {
	if selected == nil {
		return nil, errors.New("no update assets were selected")
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(workDir)
		}
	}()
	if err = os.Chmod(workDir, 0o700); err != nil {
		return nil, err
	}
	emit := func(phase string, value float64) {
		if progress != nil {
			progress(phase, clampUpdateProgress(value))
		}
	}
	emit("downloading", 0)
	checksumPath := filepath.Join(workDir, "SHA256SUMS")
	if err = downloadBounded(ctx, client, selected.Checksum.URL, checksumPath, maxChecksumBytes, nil); err != nil {
		return nil, fmt.Errorf("downloading SHA256SUMS: %w", err)
	}
	// downloadBounded closes and syncs the file before the checksum manifest is parsed.
	checksumData, err := os.ReadFile(checksumPath)
	if err != nil {
		return nil, err
	}
	if int64(len(checksumData)) > maxChecksumBytes {
		return nil, fmt.Errorf("SHA256SUMS exceeds %d bytes", maxChecksumBytes)
	}
	expected, err := parseSHA256Sums(checksumData, selected.Asset.Name)
	if err != nil {
		return nil, err
	}
	emit("downloading", 5)
	assetPath := filepath.Join(workDir, selected.Asset.Name)
	total := selected.Asset.Size
	if total <= 0 {
		total = maxUpdateAssetBytes
	}
	err = downloadBounded(ctx, client, selected.Asset.URL, assetPath, maxUpdateAssetBytes, func(done int64) {
		assetProgress := 5 + 70*float64(done)/float64(total)
		emit("downloading", min(assetProgress, 75))
	})
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", selected.Asset.Name, err)
	}
	assetInfo, err := os.Stat(assetPath)
	if err != nil {
		return nil, err
	}
	if selected.Asset.Size > 0 && assetInfo.Size() != selected.Asset.Size {
		return nil, fmt.Errorf("downloaded %d bytes for %s, expected %d", assetInfo.Size(), selected.Asset.Name, selected.Asset.Size)
	}
	emit("verifying", 80)
	if err = verifyUpdateSHA256(assetPath, expected); err != nil {
		return nil, fmt.Errorf("verifying %s: %w", selected.Asset.Name, err)
	}
	emit("verifying", 92)
	return &verifiedUpdate{WorkDir: workDir, AssetPath: assetPath, Asset: selected.Asset, SHA256: expected}, nil
}

type tarExtractionLimits struct {
	MaxBytes   int64
	MaxEntries int
}

func extractUpdateTarGz(r io.Reader, destination string, limits tarExtractionLimits) error {
	return extractUpdateTarGzContext(context.Background(), r, destination, limits)
}

type updateContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *updateContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func extractUpdateTarGzContext(ctx context.Context, r io.Reader, destination string, limits tarExtractionLimits) error {
	if limits.MaxBytes < 0 || limits.MaxEntries <= 0 {
		return errors.New("invalid archive extraction limits")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(destination)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("archive destination is not a real directory")
	}
	gz, err := gzip.NewReader(&updateContextReader{ctx: ctx, r: r})
	if err != nil {
		return err
	}
	defer gz.Close()
	gz.Multistream(false)
	limited := &io.LimitedReader{R: &updateContextReader{ctx: ctx, r: gz}, N: limits.MaxBytes + 1}
	tr := tar.NewReader(limited)
	seen := make(map[string]byte)
	entries := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		entries++
		if entries > limits.MaxEntries {
			return fmt.Errorf("archive exceeds %d entries", limits.MaxEntries)
		}
		if hdr.Size < 0 {
			return errors.New("archive entry has a negative size")
		}
		name, err := safeArchiveEntryName(hdr.Name, hdr.Typeflag)
		if err != nil {
			return err
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("archive contains duplicate entry %q", name)
		}
		seen[name] = hdr.Typeflag
		target := filepath.Join(destination, filepath.FromSlash(name))
		mode := fs.FileMode(hdr.Mode).Perm()
		switch hdr.Typeflag {
		case tar.TypeDir:
			if hdr.Size != 0 {
				return fmt.Errorf("archive directory %q has data", name)
			}
			if err := ensureArchiveParents(destination, name); err != nil {
				return err
			}
			if err := os.MkdirAll(target, mode|0o700); err != nil {
				return err
			}
			if err := os.Chmod(target, mode|0o700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if hdr.Size > limits.MaxBytes {
				return fmt.Errorf("archive exceeds %d uncompressed bytes", limits.MaxBytes)
			}
			if err := ensureArchiveParents(destination, name); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode|0o600)
			if err != nil {
				return err
			}
			written, copyErr := io.Copy(f, tr)
			if copyErr == nil && written != hdr.Size {
				copyErr = io.ErrUnexpectedEOF
			}
			if copyErr == nil {
				copyErr = f.Chmod(mode | 0o600)
			}
			closeErr := f.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("archive entry %q is a link or special file", name)
		}
	}
	// Drain through the gzip trailer so headers, padding and trailing data all
	// count toward the same decompressed-byte limit.
	if _, err := io.Copy(io.Discard, limited); err != nil {
		return err
	}
	if limited.N == 0 {
		return fmt.Errorf("archive exceeds %d uncompressed bytes", limits.MaxBytes)
	}
	return nil
}

func safeArchiveEntryName(name string, typeFlag byte) (string, error) {
	if name == "" || strings.ContainsRune(name, 0) || strings.Contains(name, `\`) || len(name) > 4096 {
		return "", fmt.Errorf("invalid archive path %q", name)
	}
	if strings.HasSuffix(name, "/") && typeFlag != tar.TypeDir {
		return "", fmt.Errorf("invalid archive file path %q", name)
	}
	clean := strings.TrimSuffix(name, "/")
	if clean == "" || path.IsAbs(name) || !fs.ValidPath(clean) || path.Clean(clean) != clean {
		return "", fmt.Errorf("archive path escapes the staging directory: %q", name)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".." || part == "." {
			return "", fmt.Errorf("archive path contains %q: %q", part, name)
		}
	}
	first, _, _ := strings.Cut(clean, "/")
	if strings.Contains(first, ":") || filepath.IsAbs(filepath.FromSlash(clean)) || filepath.VolumeName(filepath.FromSlash(clean)) != "" {
		return "", fmt.Errorf("archive path is absolute: %q", name)
	}
	return clean, nil
}

func ensureArchiveParents(destination, name string) error {
	current := destination
	parts := strings.Split(name, "/")
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("archive parent %q is not a real directory", current)
		}
	}
	return os.MkdirAll(filepath.Dir(filepath.Join(destination, filepath.FromSlash(name))), 0o700)
}

func validateExtractedLinuxPackage(directory string) error {
	executable := filepath.Join(directory, "peapod")
	info, err := os.Lstat(executable)
	if err != nil {
		return fmt.Errorf("Linux update does not contain peapod: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || (runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0) {
		return errors.New("Linux update does not contain an executable peapod binary")
	}
	return nil
}

func installVerifiedUpdate(ctx context.Context, update *verifiedUpdate, progress func(string, float64)) error {
	if update == nil {
		return errors.New("no verified update was prepared")
	}
	installed := false
	helperOwnsWorkDir := false
	defer func() {
		if !installed && !helperOwnsWorkDir {
			_ = os.RemoveAll(update.WorkDir)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	emit := func(value float64) {
		if progress != nil {
			progress("installing", value)
		}
	}
	emit(90)
	switch runtime.GOOS {
	case "windows":
		started, err := installWindowsVerifiedUpdate(update)
		if err != nil {
			helperOwnsWorkDir = started
			return err
		}
	case "linux":
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		executable, err = filepath.EvalSymlinks(executable)
		if err != nil {
			return err
		}
		target := filepath.Dir(executable)
		if err := validateLinuxUpdateTarget(target); err != nil {
			return err
		}
		if err := installLinuxArchiveContext(ctx, update.AssetPath, target); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: %s", ErrUpdateUnsupported, runtime.GOOS)
	}
	if runtime.GOOS == "linux" {
		_ = os.RemoveAll(update.WorkDir)
	}
	installed = true
	emit(100)
	return nil
}

func installLinuxArchive(archivePath, target string) error {
	return installLinuxArchiveContext(context.Background(), archivePath, target)
}

func installLinuxArchiveContext(ctx context.Context, archivePath, target string) error {
	return installLinuxArchiveWithSwapContext(ctx, archivePath, target, func(target, staging, backupRoot string) error {
		exchanged, err := exchangeLinuxDirectories(target, staging)
		if err != nil {
			return err
		}
		if exchanged {
			return nil
		}
		return swapDirectoryWithRollback(target, staging, backupRoot, os.Rename)
	})
}

func installLinuxArchiveWithRenamer(archivePath, target string, rename func(string, string) error) error {
	return installLinuxArchiveWithRenamerContext(context.Background(), archivePath, target, rename)
}

func installLinuxArchiveWithRenamerContext(ctx context.Context, archivePath, target string, rename func(string, string) error) error {
	if rename == nil {
		return errors.New("directory rename function is required")
	}
	return installLinuxArchiveWithSwapContext(ctx, archivePath, target, func(target, staging, backupRoot string) error {
		return swapDirectoryWithRollback(target, staging, backupRoot, rename)
	})
}

func installLinuxArchiveWithSwapContext(ctx context.Context, archivePath, target string, swap func(string, string, string) error) error {
	if swap == nil {
		return errors.New("directory swap function is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	parent := filepath.Dir(target)
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(target)+".staging-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	limits := tarExtractionLimits{MaxBytes: maxExtractedUpdateBytes, MaxEntries: maxExtractedUpdateEntries}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	err = extractUpdateTarGzContext(ctx, f, staging, limits)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := validateExtractedLinuxPackage(staging); err != nil {
		return err
	}
	backupRoot, err := os.MkdirTemp(parent, "."+filepath.Base(target)+".backup-")
	if err != nil {
		return err
	}
	keepBackup := false
	defer func() {
		if !keepBackup {
			_ = os.RemoveAll(backupRoot)
		}
	}()
	if err := swap(target, staging, backupRoot); err != nil {
		var rollbackErr *directoryRollbackError
		if errors.As(err, &rollbackErr) {
			// Keep the only usable copy if restoring it also failed.
			keepBackup = true
			return err
		}
		return err
	}
	// The running process keeps its old executable mapped; the path now names
	// the new version, which App.Relaunch starts after the quit sequence.
	_ = os.RemoveAll(backupRoot)
	return nil
}

type directoryRollbackError struct {
	Backup string
	Err    error
}

func (e *directoryRollbackError) Error() string {
	return fmt.Sprintf("%v; previous installation retained at %s", e.Err, e.Backup)
}

func (e *directoryRollbackError) Unwrap() error { return e.Err }

func swapDirectoryWithRollback(target, staging, backupRoot string, rename func(string, string) error) error {
	targetInfo, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		targetInfo = nil
	} else if err != nil {
		return err
	} else if !targetInfo.IsDir() || targetInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("installation target is not a real directory")
	}
	stagingInfo, err := os.Lstat(staging)
	if err != nil {
		return err
	}
	if !stagingInfo.IsDir() || stagingInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("staged update is not a real directory")
	}
	backup := filepath.Join(backupRoot, "previous")
	if _, err := os.Lstat(backup); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("installation backup already exists: %s", backup)
	}
	if targetInfo != nil {
		if err := rename(target, backup); err != nil {
			return fmt.Errorf("backing up installation: %w", err)
		}
	}
	if err := rename(staging, target); err != nil {
		if targetInfo != nil {
			if restoreErr := rename(backup, target); restoreErr != nil {
				return &directoryRollbackError{Backup: backup, Err: fmt.Errorf("installing staged update: %w; restoring installation: %v", err, restoreErr)}
			}
		}
		return fmt.Errorf("installing staged update: %w", err)
	}
	return nil
}

func validateLinuxUpdateTarget(target string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: Linux updates cannot run on %s", ErrUpdateUnsupported, runtime.GOOS)
	}
	if os.Getenv("APPIMAGE") != "" {
		return errors.New("AppImage installations cannot be updated in place")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return validateLinuxUpdateTargetAt(target, home)
}

func validateLinuxUpdateTargetAt(target, home string) error {
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return err
	}
	realTarget, err = filepath.Abs(realTarget)
	if err != nil {
		return err
	}
	realHome, err = filepath.Abs(realHome)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(realHome, realTarget)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to update %s: Linux updates require a writable user directory under %s", target, home)
	}
	for _, directory := range []string{realTarget, filepath.Dir(realTarget)} {
		probe, err := os.MkdirTemp(directory, ".peapod-update-check-")
		if err != nil {
			return fmt.Errorf("cannot update %s: %w", target, err)
		}
		if err := os.Remove(probe); err != nil {
			return err
		}
	}
	return nil
}

type windowsUpdateHelperConfig struct {
	ParentPID  int      `json:"parentPid"`
	InstallDir string   `json:"installDir"`
	Installer  string   `json:"installer"`
	Executable string   `json:"executable"`
	BackupDir  string   `json:"backupDir"`
	WorkDir    string   `json:"workDir"`
	HelperPath string   `json:"helperPath"`
	ConfigPath string   `json:"configPath"`
	ReadyFile  string   `json:"readyFile"`
	ErrorFile  string   `json:"errorFile"`
	LogPath    string   `json:"logPath"`
	WorkingDir string   `json:"workingDir"`
	Args       []string `json:"args"`
}

func installWindowsVerifiedUpdate(update *verifiedUpdate) (helperStarted bool, err error) {
	executable, err := os.Executable()
	if err != nil {
		return false, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return false, err
	}
	installDir := filepath.Dir(executable)
	backupRoot, err := os.MkdirTemp(filepath.Dir(installDir), "."+filepath.Base(installDir)+".backup-")
	if err != nil {
		return false, fmt.Errorf("cannot create an installation backup directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(backupRoot)
			_ = os.RemoveAll(update.WorkDir)
		}
	}()
	helperPath := filepath.Join(update.WorkDir, "peapod-update-helper.exe")
	if err := copyUpdateHelper(executable, helperPath); err != nil {
		return false, err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return false, err
	}
	if pathIsWithin(installDir, workingDir) {
		if err := os.Chdir(update.WorkDir); err != nil {
			return false, err
		}
		defer func() {
			if cleanup {
				_ = os.Chdir(workingDir)
			}
		}()
	}
	config := windowsUpdateHelperConfig{
		ParentPID:  os.Getpid(),
		InstallDir: installDir,
		Installer:  update.AssetPath,
		Executable: filepath.Join(installDir, "peapod.exe"),
		BackupDir:  backupRoot,
		WorkDir:    update.WorkDir,
		HelperPath: helperPath,
		ConfigPath: filepath.Join(update.WorkDir, "helper.json"),
		ReadyFile:  filepath.Join(update.WorkDir, "ready"),
		ErrorFile:  filepath.Join(update.WorkDir, "error.txt"),
		LogPath:    filepath.Join(update.WorkDir, "helper.log"),
		WorkingDir: workingDir,
		Args:       append([]string(nil), os.Args[1:]...),
	}
	data, err := json.Marshal(config)
	if err != nil {
		return false, err
	}
	f, err := os.OpenFile(config.ConfigPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return false, writeErr
	}
	if closeErr != nil {
		return false, closeErr
	}
	if err := validateWindowsHelperConfig(config); err != nil {
		return false, err
	}
	helperStarted, err = startWindowsUpdateHelper(config)
	if err != nil {
		if helperStarted {
			cleanup = false // Preserve backups if the helper may have renamed the installation.
		}
		return helperStarted, err
	}
	cleanup = false // The helper owns the download and backup until it exits.
	return true, nil
}

func copyUpdateHelper(source, destination string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	return closeErr
}

func windowsHelperStatus(config windowsUpdateHelperConfig) (bool, error) {
	data, err := os.ReadFile(config.ReadyFile)
	if err != nil {
		return false, nil
	}
	switch strings.TrimSpace(string(data)) {
	case windowsHelperComplete:
		return true, nil
	case windowsHelperFailed:
		message := "Windows update helper failed"
		if detail, readErr := os.ReadFile(config.ErrorFile); readErr == nil && strings.TrimSpace(string(detail)) != "" {
			message += ": " + strings.TrimSpace(string(detail))
		}
		return true, errors.New(message)
	default:
		return false, nil
	}
}

func startWindowsUpdateHelper(config windowsUpdateHelperConfig) (bool, error) {
	logFile, err := os.OpenFile(config.LogPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	cmd := exec.Command(config.HelperPath, updateHelperFlag, config.ConfigPath)
	cmd.Dir = config.WorkDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return false, err
	}
	_ = logFile.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(updateHelperStartupWait + updateInstallerTimeout)
	defer timer.Stop()
	for {
		if finished, statusErr := windowsHelperStatus(config); finished {
			return true, statusErr
		}
		select {
		case err := <-done:
			if err == nil {
				err = errors.New("helper exited before completing the update")
			}
			if detail, readErr := os.ReadFile(config.ErrorFile); readErr == nil && strings.TrimSpace(string(detail)) != "" {
				return true, fmt.Errorf("Windows update helper failed: %w: %s", err, strings.TrimSpace(string(detail)))
			}
			return true, fmt.Errorf("starting Windows update helper: %w", err)
		case <-ticker.C:
		case <-timer.C:
			if killErr := terminateWindowsUpdateHelperTree(cmd.Process.Pid); killErr != nil {
				_ = cmd.Process.Kill()
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
			}
			return true, errors.New("timed out waiting for Windows update helper")
		}
	}
}

// runUpdateHelper is called before the MyGo application starts. The copied
// helper therefore never creates a second native UI or touches app settings.
func runUpdateHelper(args []string) (bool, int) {
	if len(args) == 0 || args[0] != updateHelperFlag {
		return false, 0
	}
	if runtime.GOOS != "windows" {
		_, _ = fmt.Fprintln(os.Stderr, "Windows update helper cannot run on this platform")
		return true, 1
	}
	if len(args) != 2 {
		_, _ = fmt.Fprintln(os.Stderr, "update helper requires its configuration path")
		return true, 1
	}
	f, err := os.Open(args[1])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return true, 1
	}
	data, err := readBoundedReader(f, 64<<10)
	_ = f.Close()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return true, 1
	}
	var config windowsUpdateHelperConfig
	if err := json.Unmarshal(data, &config); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return true, 1
	}
	if err := runWindowsUpdateHelper(config); err != nil {
		_ = writeWindowsHelperError(config, err)
		_, _ = fmt.Fprintln(os.Stderr, err)
		return true, 1
	}
	return true, 0
}

func runWindowsUpdateHelper(config windowsUpdateHelperConfig) error {
	if err := validateWindowsHelperConfig(config); err != nil {
		return err
	}
	if config.ParentPID == os.Getpid() {
		return errors.New("invalid helper parent process")
	}
	parent, err := os.FindProcess(config.ParentPID)
	if err != nil {
		return err
	}
	// Open the parent handle before starting the installer. This avoids waiting
	// on an unrelated process if the PID is quickly reused.
	if err := os.WriteFile(config.ReadyFile, []byte(windowsHelperReady), 0o600); err != nil {
		return err
	}
	if err := replaceWindowsInstall(config.InstallDir, config.Installer, config.BackupDir, func(installer, commandLine string) error {
		ctx, cancel := context.WithTimeout(context.Background(), updateInstallerTimeout)
		defer cancel()
		output, err := executeWindowsInstallerCommand(ctx, installer, commandLine)
		if err != nil {
			if len(output) > 4096 {
				output = output[:4096]
			}
			return fmt.Errorf("NSIS installer failed: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	}); err != nil {
		_ = writeWindowsHelperError(config, err)
		_ = os.WriteFile(config.ReadyFile, []byte(windowsHelperFailed), 0o600)
		return err
	}
	if err := os.WriteFile(config.ReadyFile, []byte(windowsHelperComplete), 0o600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), updateHelperParentWait)
	defer cancel()
	if err := waitForProcessHandle(ctx, parent); err != nil {
		return fmt.Errorf("waiting for the running application to exit: %w", err)
	}
	cmd := exec.Command(config.Executable, config.Args...)
	cmd.Dir = config.WorkingDir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("update installed, but restarting Peapod failed: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		return err
	}
	// The new process owns the installation now; temporary update files and the
	// previous installation are no longer needed. Removal is best effort because
	// the helper may still be mapped.
	_ = os.RemoveAll(config.BackupDir)
	_ = os.RemoveAll(config.WorkDir)
	return nil
}

func waitForProcessHandle(ctx context.Context, process *os.Process) error {
	done := make(chan error, 1)
	go func() {
		_, err := process.Wait()
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func windowsInstallerCommandLine(installer, installDir string) string {
	// NSIS requires /D last and unquoted; it consumes the rest of the command
	// line, including spaces in the installation path.
	return quoteWindowsCommandLineArg(installer) + " /S /D=" + installDir
}

func quoteWindowsCommandLineArg(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n\v\"") {
		return value
	}
	var out strings.Builder
	out.Grow(len(value) + 2)
	out.WriteByte('"')
	backslashes := 0
	for _, r := range value {
		switch r {
		case '\\':
			backslashes++
		case '"':
			out.WriteString(strings.Repeat(`\`, backslashes*2+1))
			out.WriteByte('"')
			backslashes = 0
		default:
			out.WriteString(strings.Repeat(`\`, backslashes))
			backslashes = 0
			out.WriteRune(r)
		}
	}
	out.WriteString(strings.Repeat(`\`, backslashes*2))
	out.WriteByte('"')
	return out.String()
}

func replaceWindowsInstall(installDir, installer, backupRoot string, run func(string, string) error) error {
	if run == nil {
		return errors.New("Windows installer runner is required")
	}
	info, err := os.Stat(installDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("Windows installation target is not a directory")
	}
	installerInfo, err := os.Stat(installer)
	if err != nil {
		return err
	}
	if !installerInfo.Mode().IsRegular() || installerInfo.Size() == 0 {
		return errors.New("Windows update installer is not a regular file")
	}
	backup := filepath.Join(backupRoot, "previous")
	if _, err := os.Lstat(backup); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("Windows installation backup already exists: %s", backup)
	}
	if err := os.Rename(installDir, backup); err != nil {
		return fmt.Errorf("backing up Windows installation: %w", err)
	}
	rollback := func(installErr error) error {
		if removeErr := os.RemoveAll(installDir); removeErr != nil {
			return &directoryRollbackError{Backup: backup, Err: fmt.Errorf("%v; removing failed installation: %w", installErr, removeErr)}
		}
		if restoreErr := os.Rename(backup, installDir); restoreErr != nil {
			return &directoryRollbackError{Backup: backup, Err: fmt.Errorf("%v; restoring Windows installation: %w", installErr, restoreErr)}
		}
		_ = os.RemoveAll(backupRoot)
		return installErr
	}
	if err := run(installer, windowsInstallerCommandLine(installer, installDir)); err != nil {
		return rollback(err)
	}
	executable := filepath.Join(installDir, "peapod.exe")
	executableInfo, err := os.Lstat(executable)
	if err != nil || !executableInfo.Mode().IsRegular() || executableInfo.Size() == 0 {
		if err == nil {
			err = errors.New("installer produced no peapod.exe")
		}
		return rollback(fmt.Errorf("Windows installer did not install Peapod: %w", err))
	}
	return nil
}

func validateWindowsHelperConfig(config windowsUpdateHelperConfig) error {
	if config.ParentPID <= 0 {
		return errors.New("invalid helper parent process")
	}
	paths := []string{config.InstallDir, config.Installer, config.Executable, config.BackupDir, config.WorkDir, config.WorkingDir, config.HelperPath, config.ConfigPath, config.ReadyFile, config.ErrorFile, config.LogPath}
	for _, item := range paths {
		if item == "" || !filepath.IsAbs(item) || filepath.Clean(item) != item {
			return fmt.Errorf("invalid helper path %q", item)
		}
	}
	if filepath.Dir(config.InstallDir) == config.InstallDir {
		return errors.New("refusing to replace a filesystem root")
	}
	if config.Executable != filepath.Join(config.InstallDir, "peapod.exe") {
		return errors.New("helper executable path does not match the installation")
	}
	if filepath.Dir(config.BackupDir) != filepath.Dir(config.InstallDir) {
		return errors.New("helper backup must be on the installation volume")
	}
	for _, item := range []string{config.HelperPath, config.ConfigPath, config.ReadyFile, config.ErrorFile, config.LogPath, config.Installer} {
		if filepath.Dir(item) != config.WorkDir {
			return fmt.Errorf("helper file %q is outside its working directory", item)
		}
	}
	if pathIsWithin(config.WorkDir, config.InstallDir) || pathIsWithin(config.InstallDir, config.WorkDir) {
		return errors.New("helper working directory overlaps the installation")
	}
	realInstallDir, err := filepath.EvalSymlinks(config.InstallDir)
	if err != nil {
		return err
	}
	if !samePath(realInstallDir, config.InstallDir) {
		return errors.New("refusing to replace a symlinked installation directory")
	}
	for _, item := range []string{config.HelperPath, config.Installer} {
		info, err := os.Lstat(item)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("helper file %q is not a regular file", item)
		}
	}
	return nil
}

func pathIsWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func writeWindowsHelperError(config windowsUpdateHelperConfig, err error) error {
	if config.ErrorFile == "" {
		return nil
	}
	return os.WriteFile(config.ErrorFile, []byte(err.Error()+"\n"), 0o600)
}

var restartUpdatedApplication = func(goos string) {
	if goos == "windows" {
		// The helper waits for this process to exit, installs and restarts.
		mygo.App.Quit()
		return
	}
	mygo.App.Relaunch()
}
