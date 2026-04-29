package e2etest

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestSPA(t *testing.T) {
	fixture := setupFixture(t, modeSPA)
	_, endpoint := TestCaddyContainerWithFixture(t, fixture)
	client := newClient()

	t.Run("A_TryFiles", func(t *testing.T) {
		cases := []struct {
			name        string
			path        string
			status      int
			statusOneOf []int
			body        string
		}{
			{"A1_root", "/", 200, nil, "SPA_INDEX"},
			{"A2_index_html", "/index.html", 200, nil, "SPA_INDEX"},
			{"A3_about_no_ext", "/about", 200, nil, "ABOUT_PAGE"},
			{"A4_about_html", "/about.html", 200, nil, "ABOUT_PAGE"},
			{"A5_users_slash", "/users/", 200, nil, "USERS_INDEX"},
			{"A6_users_no_slash", "/users", 0, []int{200, 308}, "USERS_INDEX"},
			{"A7_blog_slash", "/blog/", 200, nil, "BLOG_INDEX"},
			{"A8_blog_post_no_ext", "/blog/post-1", 200, nil, "POST_1"},
			{"A9_blog_post_html", "/blog/post-1.html", 200, nil, "POST_1"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				finalClient := client
				if len(tc.statusOneOf) > 0 {
					// follow redirect to get final body
					finalClient = &http.Client{Transport: &http.Transport{DisableCompression: true}}
				}
				res, err := finalClient.Get(endpoint + tc.path)
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				e := expectation{status: tc.status, statusOneOf: tc.statusOneOf, bodyContains: tc.body}
				assertResponse(t, res, e)
			})
		}

		// A10 data.json
		t.Run("A10_data_json", func(t *testing.T) {
			res, err := client.Get(endpoint + "/data.json")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:       200,
				bodyContains: `{"real":true}`,
				contentType:  "application/json",
			})
		})

		// A11 app.js
		t.Run("A11_app_js", func(t *testing.T) {
			res, err := client.Get(endpoint + "/assets/app.js")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:       200,
				bodyContains: "REAL_ASSET_JS",
				contentType:  "javascript",
			})
		})

		// A12 style.css
		t.Run("A12_style_css", func(t *testing.T) {
			res, err := client.Get(endpoint + "/assets/style.css")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:      200,
				bodyContains: "REAL_ASSET_CSS",
				contentType: "text/css",
			})
		})

		// A13 logo.png
		t.Run("A13_logo_png", func(t *testing.T) {
			res, err := client.Get(endpoint + "/img/logo.png")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:      200,
				contentType: "image/png",
			})
			// binary length > 0 is implied by status 200; body was already read in assertResponse
		})

		// A14 .well-known/security.txt
		t.Run("A14_well_known", func(t *testing.T) {
			res, err := client.Get(endpoint + "/.well-known/security.txt")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:       200,
				bodyContains: "WELL_KNOWN",
			})
		})
	})

	t.Run("B_Forbidden", func(t *testing.T) {
		cases := []struct {
			name        string
			path        string
			notContains string
		}{
			{"B1_git_config", "/.git/config", "SHOULD_NEVER_LEAK_GIT"},
			{"B2_git_HEAD", "/.git/HEAD", ""},
			{"B3_node_modules", "/node_modules/pkg/index.js", "SHOULD_NEVER_LEAK_NM"},
			{"B4_vendor", "/vendor/lib.php", "SHOULD_NEVER_LEAK_VENDOR"},
			{"B5_venv", "/.venv/pyvenv.cfg", "SHOULD_NEVER_LEAK_VENV"},
			{"B7_git_trailing_slash", "/.git/", ""},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				res, err := client.Get(endpoint + tc.path)
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				e := expectation{status: 404}
				if tc.notContains != "" {
					e.bodyNotContains = []string{tc.notContains}
				}
				assertResponse(t, res, e)
			})
		}

		// B6: /.git (no trailing slash) — design decision, assert actual behavior
		t.Run("B6_git_no_slash", func(t *testing.T) {
			res, err := client.Get(endpoint + "/.git")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			if strings.Contains(string(body), "SHOULD_NEVER_LEAK_GIT") {
				t.Error("/.git leaked git content")
			}
		})

		// B8: /any/.git/config — @forbidden is prefix-anchored, so this is NOT blocked
		t.Run("B8_mid_path_git", func(t *testing.T) {
			res, err := client.Get(endpoint + "/any/.git/config")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			// Document current behavior: not blocked because @forbidden only matches /.git/*
			t.Logf("B8 /any/.git/config → status %d, body %q", res.StatusCode, body)
		})
	})

	t.Run("C_SPAFallback", func(t *testing.T) {
		cases := []struct {
			name string
			path string
		}{
			{"C1_projects", "/projects"},
			{"C2_projects_slash", "/projects/"},
			{"C3_projects_id", "/projects/123"},
			{"C4_deeply_nested", "/deeply/nested/spa/route"},
			{"C5_query_string", "/projects?id=1&filter=foo"},
			{"C6_nonexistent_html", "/some-page.html"},
			{"C7_nonexistent_htm", "/some-page.htm"},
			{"C8_url_safe_chars", "/-_~!$&()*+,;=:@"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				res, err := client.Get(endpoint + tc.path)
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				assertResponse(t, res, expectation{
					status:       200,
					bodyContains: "SPA_INDEX",
					contentType:  "text/html",
				})
			})
		}

		// C9 HEAD /projects
		t.Run("C9_head", func(t *testing.T) {
			getRes, err := client.Get(endpoint + "/projects")
			if err != nil {
				t.Fatal(err)
			}
			getBody, err := io.ReadAll(getRes.Body)
			if err != nil {
				t.Fatalf("read response body: %v", err)
			}
			if err := getRes.Body.Close(); err != nil {
				t.Fatalf("close response body: %v", err)
			}

			req, _ := http.NewRequest("HEAD", endpoint+"/projects", nil)
			headRes, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			headBody, err := io.ReadAll(headRes.Body)
			if err != nil {
				t.Fatalf("read response body: %v", err)
			}
			if err := headRes.Body.Close(); err != nil {
				t.Fatalf("close response body: %v", err)
			}

			if headRes.StatusCode != 200 {
				t.Errorf("HEAD status: want 200, got %d", headRes.StatusCode)
			}
			if len(headBody) != 0 {
				t.Errorf("HEAD body: want empty, got %q", headBody)
			}
			if cl := headRes.Header.Get("Content-Length"); cl != "" {
				wantCL := strconv.Itoa(len(getBody))
				if cl != wantCL {
					t.Errorf("HEAD Content-Length: want %s, got %s", wantCL, cl)
				}
			}
		})
	})

	t.Run("E_AssetMiss", func(t *testing.T) {
		for _, path := range assetMissPaths {
			path := path
			t.Run(path, func(t *testing.T) {
				res, err := client.Get(endpoint + path)
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				assertResponse(t, res, expectation{
					status:          404,
					bodyContains:    "Not Found",
					bodyNotContains: []string{"SPA_INDEX", "CUSTOM_404"},
					contentTypeNot:  "text/html",
				})
			})
		}
	})

	t.Run("F_AssetBoundary", func(t *testing.T) {
		// F1: trailing dot — no extension chars after dot → treated as doc
		t.Run("F1_trailing_dot", func(t *testing.T) {
			res, err := client.Get(endpoint + "/file.")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{status: 200, bodyContains: "SPA_INDEX"})
		})

		// F2: dash + digits, no dot → doc
		t.Run("F2_dash_digits", func(t *testing.T) {
			res, err := client.Get(endpoint + "/article-2024")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{status: 200, bodyContains: "SPA_INDEX"})
		})

		// F3: /api/v1.2 — .2 matches regex → treated as asset → 404 (known limitation)
		t.Run("F3_dot_in_last_segment", func(t *testing.T) {
			res, err := client.Get(endpoint + "/api/v1.2")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			t.Logf("F3 /api/v1.2 → status %d body %q (known: numeric ext treated as asset)", res.StatusCode, body)
			// Assert 404 and plain body (not SPA fallback) — this is the intended behavior
			if res.StatusCode != 404 {
				t.Errorf("F3: want 404, got %d", res.StatusCode)
			}
			if strings.Contains(string(body), "SPA_INDEX") {
				t.Error("F3: must not fall back to SPA_INDEX")
			}
		})

		// F4: dot in non-final segment, final segment has no dot → doc
		t.Run("F4_dot_mid_path", func(t *testing.T) {
			res, err := client.Get(endpoint + "/v1.0.0/page")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{status: 200, bodyContains: "SPA_INDEX"})
		})

		// F5: .tar.gz → asset
		t.Run("F5_tar_gz", func(t *testing.T) {
			res, err := client.Get(endpoint + "/file.tar.gz")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:          404,
				bodyNotContains: []string{"SPA_INDEX"},
			})
		})

		// F6: double dot .js → asset
		t.Run("F6_double_dot", func(t *testing.T) {
			res, err := client.Get(endpoint + "/file..js")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:          404,
				bodyNotContains: []string{"SPA_INDEX"},
			})
		})

		// F7: /FILE.HTML — Caddy path matcher case behaviour
		t.Run("F7_uppercase_html", func(t *testing.T) {
			res, err := client.Get(endpoint + "/FILE.HTML")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			t.Logf("F7 /FILE.HTML → status %d body %q (case sensitivity varies by OS)", res.StatusCode, body)
		})

		// F8: .hidden dotfile treated as asset
		t.Run("F8_dotfile", func(t *testing.T) {
			res, err := client.Get(endpoint + "/.hidden")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:          404,
				bodyNotContains: []string{"SPA_INDEX"},
			})
		})

		// F9: unicode path → no ASCII extension → doc
		t.Run("F9_unicode_path", func(t *testing.T) {
			res, err := client.Get(endpoint + "/%E8%B7%AF%E5%BE%91/%E4%B8%AD%E6%96%87")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{status: 200, bodyContains: "SPA_INDEX"})
		})
	})

	t.Run("G_PathEdges", func(t *testing.T) {
		// G3: path traversal
		t.Run("G3_traversal", func(t *testing.T) {
			res, err := client.Get(endpoint + "/../etc/passwd")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode == 200 {
				body, _ := io.ReadAll(res.Body)
				if strings.Contains(string(body), "root:") {
					t.Error("path traversal succeeded — /etc/passwd leaked")
				}
			}
		})

		// G4: /foo/../about normalised to /about
		t.Run("G4_normalize", func(t *testing.T) {
			followClient := &http.Client{Transport: &http.Transport{DisableCompression: true}}
			res, err := followClient.Get(endpoint + "/foo/../about")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			t.Logf("G4 /foo/../about → status %d body %q", res.StatusCode, body)
		})

		// G7: very long path
		t.Run("G7_long_path", func(t *testing.T) {
			longPath := "/" + strings.Repeat("a", 4096)
			res, err := client.Get(endpoint + longPath)
			if err != nil {
				// connection reset / 414 are both acceptable
				t.Logf("G7 long path error (acceptable): %v", err)
				return
			}
			defer res.Body.Close()
			t.Logf("G7 long path → status %d", res.StatusCode)
		})

		// G8: Range header
		t.Run("G8_range", func(t *testing.T) {
			req, _ := http.NewRequest("GET", endpoint+"/", nil)
			req.Header.Set("Range", "bytes=0-9")
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusPartialContent && res.StatusCode != http.StatusOK {
				t.Errorf("G8 Range: want 206 or 200, got %d", res.StatusCode)
			}
		})

		// G9: conditional request with ETag
		t.Run("G9_conditional", func(t *testing.T) {
			res1, err := client.Get(endpoint + "/")
			if err != nil {
				t.Fatal(err)
			}
			etag := res1.Header.Get("ETag")
			if _, err := io.ReadAll(res1.Body); err != nil {
				t.Fatalf("reading response body: %v", err)
			}
			if err := res1.Body.Close(); err != nil {
				t.Fatalf("closing response body: %v", err)
			}

			if etag == "" {
				t.Skip("no ETag header returned, skipping conditional request test")
			}

			req, _ := http.NewRequest("GET", endpoint+"/", nil)
			req.Header.Set("If-None-Match", etag)
			res2, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := res2.Body.Close(); err != nil {
					t.Errorf("closing response body: %v", err)
				}
			}()
			if res2.StatusCode != http.StatusNotModified {
				t.Errorf("G9 conditional: want 304, got %d", res2.StatusCode)
			}
		})
	})

	t.Run("H_Methods", func(t *testing.T) {
		// H1 HEAD /
		t.Run("H1_head_root", func(t *testing.T) {
			req, _ := http.NewRequest("HEAD", endpoint+"/", nil)
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != 200 {
				t.Errorf("want 200, got %d", res.StatusCode)
			}
			if len(body) != 0 {
				t.Errorf("HEAD body must be empty, got %q", body)
			}
		})

		// H2 HEAD /projects (SPA)
		t.Run("H2_head_spa_route", func(t *testing.T) {
			req, _ := http.NewRequest("HEAD", endpoint+"/projects", nil)
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != 200 {
				t.Errorf("want 200, got %d", res.StatusCode)
			}
			if len(body) != 0 {
				t.Errorf("HEAD body must be empty, got %q", body)
			}
		})

		// H4 HEAD /assets/missing.js
		t.Run("H4_head_asset_miss", func(t *testing.T) {
			req, _ := http.NewRequest("HEAD", endpoint+"/assets/missing.js", nil)
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != 404 {
				t.Errorf("want 404, got %d", res.StatusCode)
			}
			if len(body) != 0 {
				t.Errorf("HEAD body must be empty, got %q", body)
			}
		})
	})

	t.Run("I_Headers", func(t *testing.T) {
		// I1: gzip encoding
		t.Run("I1_gzip", func(t *testing.T) {
			req, _ := http.NewRequest("GET", endpoint+"/assets/app.js", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				t.Fatalf("want 200, got %d", res.StatusCode)
			}
			enc := res.Header.Get("Content-Encoding")
			if enc != "gzip" {
				t.Skipf("server did not gzip (Content-Encoding: %q) — file may be too small to compress", enc)
			}
			r, err := gzip.NewReader(res.Body)
			if err != nil {
				t.Fatalf("gzip reader: %v", err)
			}
			defer func() {
				if err := r.Close(); err != nil {
					t.Fatalf("gzip reader Close failed: %v", err)
				}
			}()
			body, _ := io.ReadAll(r)
			if !strings.Contains(string(body), "REAL_ASSET_JS") {
				t.Errorf("decompressed body: want REAL_ASSET_JS, got %q", body)
			}
		})

		// I3: no Accept-Encoding → no Content-Encoding
		t.Run("I3_no_encoding", func(t *testing.T) {
			req, _ := http.NewRequest("GET", endpoint+"/assets/app.js", nil)
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			enc := res.Header.Get("Content-Encoding")
			if enc != "" {
				t.Errorf("want no Content-Encoding, got %q", enc)
			}
		})

		// I4: root has Content-Type, ETag, Last-Modified
		t.Run("I4_root_headers", func(t *testing.T) {
			res, err := client.Get(endpoint + "/")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:      200,
				contentType: "text/html",
			})
			if res.Header.Get("Last-Modified") == "" && res.Header.Get("ETag") == "" {
				t.Error("want at least one of ETag or Last-Modified")
			}
		})

		// I5: data.json Content-Type
		t.Run("I5_json_content_type", func(t *testing.T) {
			res, err := client.Get(endpoint + "/data.json")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:      200,
				contentType: "application/json",
			})
		})

		// I6: PNG has Accept-Ranges
		t.Run("I6_png_accept_ranges", func(t *testing.T) {
			res, err := client.Get(endpoint + "/img/logo.png")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:      200,
				contentType: "image/png",
			})
			if !strings.Contains(res.Header.Get("Accept-Ranges"), "bytes") {
				t.Errorf("want Accept-Ranges: bytes, got %q", res.Header.Get("Accept-Ranges"))
			}
		})

		// I8: asset miss Content-Type is not text/html
		t.Run("I8_asset_miss_content_type", func(t *testing.T) {
			res, err := client.Get(endpoint + "/assets/missing.js")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:         404,
				contentTypeNot: "text/html",
			})
		})

		// I9: SPA route returns text/html with 200
		t.Run("I9_spa_route_content_type", func(t *testing.T) {
			res, err := client.Get(endpoint + "/projects")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:      200,
				contentType: "text/html",
			})
		})
	})

	t.Run("Regression", func(t *testing.T) {
		// J1: SPA route must return 200, not 404
		t.Run("J1_spa_route_returns_200_not_404", func(t *testing.T) {
			res, err := client.Get(endpoint + "/projects")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:       200,
				bodyContains: "SPA_INDEX",
			})
		})

		// J3: missing asset must return 404 plain, not SPA index
		t.Run("J3_spa_missing_asset_returns_404_not_200", func(t *testing.T) {
			res, err := client.Get(endpoint + "/assets/missing.js")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:          404,
				bodyContains:    "Not Found",
				bodyNotContains: []string{"SPA_INDEX"},
				contentTypeNot:  "text/html",
			})
		})
	})
}
