package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		left, right string
		want        int
	}{
		{"0.3.1", "0.3.0", 1},
		{"v1.0.0", "1.0.0-dev", 0},
		{"0.2.9", "0.3.0", -1},
	}
	for _, test := range tests {
		if got := compareVersions(test.left, test.right); got != test.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestCheckSelectsWindowsAssetAndChecksums(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("Windows asset selection is platform-specific")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{
          "tag_name":"v0.3.1","name":"USBridge 0.3.1","body":"Fixes",
          "html_url":"https://example.test/release","published_at":"2026-08-31T00:00:00Z",
          "assets":[
            {"name":"USBridge-Windows-x64-0.3.1-unsigned.exe","browser_download_url":"https://example.test/app.exe","size":42},
            {"name":"SHA256SUMS.txt","browser_download_url":"https://example.test/sums","size":100}
          ]}`)
	}))
	defer server.Close()
	client := &Client{HTTPClient: server.Client(), APIURL: server.URL}
	info, err := client.Check(context.Background(), "0.3.0")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || info.LatestVersion != "0.3.1" || info.asset.Name == "" || info.checksums.Name == "" {
		t.Fatalf("unexpected update info: %+v", info)
	}
}

func TestChecksumForExactAsset(t *testing.T) {
	contents := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  first.exe\n" +
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb *USBridge.exe\n")
	value, err := checksumFor(contents, "USBridge.exe")
	if err != nil {
		t.Fatal(err)
	}
	if value != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("unexpected checksum: %s", value)
	}
}

func TestDownloadVerifiesChecksumAndExecutableHeader(t *testing.T) {
	payload := []byte("MZtest executable")
	digest := sha256.Sum256(payload)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/sums":
			fmt.Fprintf(writer, "%s  USBridge.exe\n", hex.EncodeToString(digest[:]))
		case "/app.exe":
			_, _ = writer.Write(payload)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client := &Client{HTTPClient: server.Client()}
	path, err := client.Download(context.Background(), Info{
		Available: true,
		asset:     Asset{Name: "USBridge.exe", URL: server.URL + "/app.exe", Size: int64(len(payload))},
		checksums: Asset{Name: "SHA256SUMS.txt", URL: server.URL + "/sums"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Dir(path))
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != string(payload) {
		t.Fatalf("downloaded payload = %q", contents)
	}
}
