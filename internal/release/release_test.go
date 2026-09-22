package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVersionContract(t *testing.T) {
	for _, tag := range []string{"v0.1.0", "v12.34.56"} {
		if !ValidVersion(tag) {
			t.Errorf("rejected %q", tag)
		}
	}
	for _, tag := range []string{"0.1.0", "v1.2", "v01.2.3", "v1.2.3-beta"} {
		if ValidVersion(tag) {
			t.Errorf("accepted %q", tag)
		}
	}
	if cmp, err := Compare("v1.9.0", "v1.10.0"); err != nil || cmp >= 0 {
		t.Fatalf("Compare = %d, %v", cmp, err)
	}
}

func TestDownloadVerifiesChecksumAndExtractsBinary(t *testing.T) {
	const tag = "v1.2.3"
	name, err := AssetName(tag, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	const binary = "verified binary"
	if err := tw.WriteHeader(&tar.Header{Name: "crwai", Mode: 0o755, Size: int64(len(binary)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(binary)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(archive.Bytes())
	checksum := hex.EncodeToString(hash[:])
	validChecksum := true
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body []byte
		status := http.StatusOK
		switch r.URL.Path {
		case "/latest", "/tags/" + tag:
			body = []byte(fmt.Sprintf(`{"tag_name":%q}`, tag))
		case "/" + tag + "/checksums.txt":
			if validChecksum {
				body = []byte(fmt.Sprintf("%s  %s\n", checksum, name))
			} else {
				body = []byte(fmt.Sprintf("%064d  %s\n", 0, name))
			}
		case "/" + tag + "/" + name:
			body = archive.Bytes()
		default:
			status = http.StatusNotFound
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
	client := Client{HTTP: httpClient, APIBase: "https://example.test", DownloadBase: "https://example.test"}
	if got, err := client.Latest(context.Background()); err != nil || got != tag {
		t.Fatalf("Latest = %q, %v", got, err)
	}
	if got, err := client.Version(context.Background(), tag); err != nil || got != tag {
		t.Fatalf("Version = %q, %v", got, err)
	}
	if _, err := client.Version(context.Background(), "v9.9.9"); !errors.Is(err, ErrReleaseNotFound) {
		t.Fatalf("missing release error = %v", err)
	}
	got, err := client.Download(context.Background(), tag)
	if err != nil || string(got) != binary {
		t.Fatalf("Download = %q, %v", got, err)
	}
	validChecksum = false
	if _, err := client.Download(context.Background(), tag); err == nil {
		t.Fatal("accepted a package with the wrong checksum")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestReplaceInstallsExecutable(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bin", "crwai")
	if err := Replace(target, []byte("new binary")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "new binary" {
		t.Fatalf("installed binary = %q, %v", got, err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed file is not executable: %v, %v", info, err)
	}
}
