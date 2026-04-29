package e2etest

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

const (
	modeSPA = "spa"
	modeMPA = "mpa"
)

type expectation struct {
	status          int
	statusOneOf     []int
	bodyContains    string
	bodyNotContains []string
	contentType     string
	contentTypeNot  string
}

func assertResponse(t *testing.T, res *http.Response, e expectation) {
	t.Helper()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodyStr := string(body)
	ct := res.Header.Get("Content-Type")

	if e.status != 0 {
		if res.StatusCode != e.status {
			t.Errorf("status: want %d, got %d (body: %q)", e.status, res.StatusCode, bodyStr)
		}
	}
	if len(e.statusOneOf) > 0 {
		found := false
		for _, s := range e.statusOneOf {
			if res.StatusCode == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("status: want one of %v, got %d (body: %q)", e.statusOneOf, res.StatusCode, bodyStr)
		}
	}
	if e.bodyContains != "" {
		if !strings.Contains(bodyStr, e.bodyContains) {
			t.Errorf("body: want to contain %q, got %q", e.bodyContains, bodyStr)
		}
	}
	for _, notContain := range e.bodyNotContains {
		if strings.Contains(bodyStr, notContain) {
			t.Errorf("body: must NOT contain %q, but it does (body: %q)", notContain, bodyStr)
		}
	}
	if e.contentType != "" {
		if !strings.Contains(ct, e.contentType) {
			t.Errorf("Content-Type: want %q to contain %q", ct, e.contentType)
		}
	}
	if e.contentTypeNot != "" {
		if strings.Contains(ct, e.contentTypeNot) {
			t.Errorf("Content-Type: want %q NOT to contain %q", ct, e.contentTypeNot)
		}
	}
}

// newClient returns a client that does not follow redirects and does not
// decompress responses so that Content-Encoding headers remain visible.
func newClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{DisableCompression: true},
	}
}

// setupFixture returns the absolute path to the fixture directory for the given
// mode ("spa" or "mpa"). The inner directory is named "caddy" so testcontainers'
// tarDir creates entries prefixed with "caddy/", which Docker extracts to the
// correct /usr/share/caddy path (testcontainers copies to the parent of
// ContainerFilePath and uses the source directory name as the TAR prefix).
func setupFixture(t *testing.T, mode string) string {
	t.Helper()
	dir, err := filepath.Abs("testdata/" + mode + "/caddy")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// assetMissPaths is the full E1-E20 list of missing asset paths.
var assetMissPaths = []string{
	"/assets/missing.js",
	"/assets/missing.mjs",
	"/assets/missing.css",
	"/assets/missing.css.map",
	"/img/missing.png",
	"/img/missing.jpg",
	"/img/missing.svg",
	"/img/missing.webp",
	"/img/missing.avif",
	"/img/missing.ico",
	"/fonts/missing.woff2",
	"/fonts/missing.woff",
	"/fonts/missing.ttf",
	"/missing.json",
	"/missing.xml",
	"/missing.txt",
	"/missing.pdf",
	"/missing.mp4",
	"/missing.wasm",
	"/missing.zip",
}
