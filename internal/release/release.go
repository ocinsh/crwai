// Package release downloads and verifies published crwai binaries.
package release

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
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPI      = "https://api.github.com/repos/ocinsh/crwai/releases"
	defaultDownload = "https://github.com/ocinsh/crwai/releases/download"
	maxArchive      = 100 << 20
)

var versionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// ErrReleaseNotFound means GitHub has no matching published release.
var ErrReleaseNotFound = errors.New("release not found")

// ValidVersion reports whether tag follows the release vX.X.X contract.
func ValidVersion(tag string) bool { return versionPattern.MatchString(tag) }

// Compare returns -1, 0, or 1 when a is older than, equal to, or newer than b.
func Compare(a, b string) (int, error) {
	if !ValidVersion(a) || !ValidVersion(b) {
		return 0, errors.New("versions must use vX.X.X")
	}
	left := strings.Split(strings.TrimPrefix(a, "v"), ".")
	right := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := range left {
		x, err := strconv.ParseUint(left[i], 10, 64)
		if err != nil {
			return 0, err
		}
		y, err := strconv.ParseUint(right[i], 10, 64)
		if err != nil {
			return 0, err
		}
		if x < y {
			return -1, nil
		}
		if x > y {
			return 1, nil
		}
	}
	return 0, nil
}

// Client addresses the official GitHub release endpoints.
type Client struct {
	HTTP         *http.Client
	APIBase      string
	DownloadBase string
}

// NewClient returns a client with bounded HTTP requests and official endpoints.
func NewClient() Client {
	return Client{HTTP: &http.Client{Timeout: 30 * time.Second}, APIBase: defaultAPI, DownloadBase: defaultDownload}
}

// Latest fetches the most recent published stable release tag.
func (c Client) Latest(ctx context.Context) (string, error) {
	return c.fetchTag(ctx, c.APIBase+"/latest")
}

// Version confirms a specific release tag exists on GitHub.
func (c Client) Version(ctx context.Context, tag string) (string, error) {
	if !ValidVersion(tag) {
		return "", fmt.Errorf("invalid version %q: expected vX.X.X", tag)
	}
	return c.fetchTag(ctx, c.APIBase+"/tags/"+tag)
}

func (c Client) fetchTag(ctx context.Context, url string) (string, error) {
	data, err := c.get(ctx, url, 1<<20)
	if err != nil {
		return "", err
	}
	var item struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(data, &item); err != nil {
		return "", fmt.Errorf("decode release: %w", err)
	}
	if !ValidVersion(item.TagName) {
		return "", fmt.Errorf("release has invalid tag %q", item.TagName)
	}
	return item.TagName, nil
}

// AssetName returns the package name for a supported operating system and CPU.
func AssetName(tag, goos, goarch string) (string, error) {
	if !ValidVersion(tag) {
		return "", fmt.Errorf("invalid version %q", tag)
	}
	if goos != "linux" && goos != "darwin" {
		return "", fmt.Errorf("unsupported operating system %q", goos)
	}
	if goarch != "amd64" && goarch != "arm64" {
		return "", fmt.Errorf("unsupported architecture %q", goarch)
	}
	return fmt.Sprintf("crwai_%s_%s_%s.tar.gz", tag, goos, goarch), nil
}

// Download verifies the archive checksum and extracts its crwai executable.
func (c Client) Download(ctx context.Context, tag string) ([]byte, error) {
	name, err := AssetName(tag, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	base := c.DownloadBase + "/" + tag + "/"
	checksums, err := c.get(ctx, base+"checksums.txt", 1<<20)
	if err != nil {
		return nil, fmt.Errorf("download checksums: %w", err)
	}
	want, err := checksumFor(checksums, name)
	if err != nil {
		return nil, err
	}
	archive, err := c.get(ctx, base+name, maxArchive)
	if err != nil {
		return nil, fmt.Errorf("download package: %w", err)
	}
	got := sha256.Sum256(archive)
	if !bytes.Equal(got[:], want) {
		return nil, errors.New("package checksum mismatch")
	}
	return unpack(archive)
}

func checksumFor(data []byte, name string) ([]byte, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			checksum, err := hex.DecodeString(fields[0])
			if err != nil || len(checksum) != sha256.Size {
				return nil, errors.New("invalid package checksum")
			}
			return checksum, nil
		}
	}
	return nil, fmt.Errorf("checksum missing for %s", name)
}

func unpack(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("package has no crwai binary")
		}
		if err != nil {
			return nil, err
		}
		if h.Name != "crwai" || h.Typeflag != tar.TypeReg {
			continue
		}
		if h.Size < 1 || h.Size > maxArchive {
			return nil, errors.New("invalid crwai binary size")
		}
		return io.ReadAll(io.LimitReader(tr, maxArchive))
	}
}

func (c Client) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "crwai-update")
	client := c.HTTP
	if client == nil {
		client = NewClient().HTTP
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, ErrReleaseNotFound
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned HTTP %d for %s", res.StatusCode, url)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("release response exceeds size limit")
	}
	return data, nil
}

// Replace atomically installs an executable at destination.
func Replace(destination string, executable []byte) error {
	if len(executable) == 0 {
		return errors.New("empty executable")
	}
	dir := filepath.Dir(destination)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".crwai-install-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(executable); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), destination)
}
