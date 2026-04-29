package e2etest

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMPA(t *testing.T) {
	fixture := setupFixture(t, modeMPA)
	_, endpoint := TestCaddyContainerWithFixture(t, fixture)
	client := newClient()

	t.Run("A_TryFiles", func(t *testing.T) {
		cases := []struct {
			name        string
			path        string
			status      int
			statusOneOf []int
			body        string
			ct          string
		}{
			{"A1_root", "/", 200, nil, "INDEX_CONTENT", "text/html"},
			{"A2_index_html", "/index.html", 200, nil, "INDEX_CONTENT", "text/html"},
			{"A3_about_no_ext", "/about", 200, nil, "ABOUT_PAGE", "text/html"},
			{"A4_about_html", "/about.html", 200, nil, "ABOUT_PAGE", "text/html"},
			{"A5_users_slash", "/users/", 200, nil, "USERS_INDEX", "text/html"},
			{"A6_users_no_slash", "/users", 0, []int{200, 308}, "USERS_INDEX", ""},
			{"A7_blog_slash", "/blog/", 200, nil, "BLOG_INDEX", "text/html"},
			{"A8_blog_post_no_ext", "/blog/post-1", 200, nil, "POST_1", "text/html"},
			{"A9_blog_post_html", "/blog/post-1.html", 200, nil, "POST_1", "text/html"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				finalClient := client
				if len(tc.statusOneOf) > 0 {
					finalClient = &http.Client{Transport: &http.Transport{DisableCompression: true}}
				}
				res, err := finalClient.Get(endpoint + tc.path)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := res.Body.Close(); err != nil {
						t.Fatalf("closing response body: %v", err)
					}
				}()
				assertResponse(t, res, expectation{
					status:       tc.status,
					statusOneOf:  tc.statusOneOf,
					bodyContains: tc.body,
					contentType:  tc.ct,
				})
			})
		}

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

		t.Run("A12_style_css", func(t *testing.T) {
			res, err := client.Get(endpoint + "/assets/style.css")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:       200,
				bodyContains: "REAL_ASSET_CSS",
				contentType:  "text/css",
			})
		})

		t.Run("A13_logo_png", func(t *testing.T) {
			res, err := client.Get(endpoint + "/img/logo.png")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{status: 200, contentType: "image/png"})
		})

		t.Run("A14_well_known", func(t *testing.T) {
			res, err := client.Get(endpoint + "/.well-known/security.txt")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{status: 200, bodyContains: "WELL_KNOWN"})
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

		t.Run("B8_mid_path_git", func(t *testing.T) {
			res, err := client.Get(endpoint + "/any/.git/config")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			t.Logf("B8 /any/.git/config → status %d body %q", res.StatusCode, body)
		})
	})

	t.Run("D_MPAFallback", func(t *testing.T) {
		cases := []struct {
			name string
			path string
		}{
			{"D1_projects", "/projects"},
			{"D2_projects_id", "/projects/123"},
			{"D3_deeply_nested", "/deeply/nested/missing"},
			{"D4_missing_html", "/missing.html"},
			{"D5_missing_htm", "/missing.htm"},
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
					status:       404,
					bodyContains: "CUSTOM_404",
					contentType:  "text/html",
				})
			})
		}

		// D6: direct request to /404.html itself — should be 200 (real file hit)
		t.Run("D6_direct_404_html", func(t *testing.T) {
			res, err := client.Get(endpoint + "/404.html")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:       200,
				bodyContains: "CUSTOM_404",
			})
		})

		// D8: HEAD /projects → 404 empty body
		t.Run("D8_head_mpa_route", func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), "HEAD", endpoint+"/projects", nil)
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
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
					bodyNotContains: []string{"INDEX_CONTENT", "CUSTOM_404"},
					contentTypeNot:  "text/html",
				})
			})
		}
	})

	t.Run("F_AssetBoundary", func(t *testing.T) {
		t.Run("F1_trailing_dot", func(t *testing.T) {
			res, err := client.Get(endpoint + "/file.")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{status: 404, bodyContains: "CUSTOM_404"})
		})

		t.Run("F2_dash_digits", func(t *testing.T) {
			res, err := client.Get(endpoint + "/article-2024")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{status: 404, bodyContains: "CUSTOM_404"})
		})

		t.Run("F3_dot_in_last_segment", func(t *testing.T) {
			res, err := client.Get(endpoint + "/api/v1.2")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			t.Logf("F3 /api/v1.2 → status %d body %q (known: numeric ext treated as asset)", res.StatusCode, body)
			if res.StatusCode != 404 {
				t.Errorf("F3: want 404, got %d", res.StatusCode)
			}
			if strings.Contains(string(body), "CUSTOM_404") {
				t.Error("F3: asset miss must not fall back to CUSTOM_404")
			}
		})

		t.Run("F4_dot_mid_path", func(t *testing.T) {
			res, err := client.Get(endpoint + "/v1.0.0/page")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{status: 404, bodyContains: "CUSTOM_404"})
		})

		t.Run("F5_tar_gz", func(t *testing.T) {
			res, err := client.Get(endpoint + "/file.tar.gz")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:          404,
				bodyNotContains: []string{"CUSTOM_404"},
			})
		})

		t.Run("F6_double_dot", func(t *testing.T) {
			res, err := client.Get(endpoint + "/file..js")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:          404,
				bodyNotContains: []string{"CUSTOM_404"},
			})
		})

		t.Run("F7_uppercase_html", func(t *testing.T) {
			res, err := client.Get(endpoint + "/FILE.HTML")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			t.Logf("F7 /FILE.HTML → status %d body %q (case sensitivity varies)", res.StatusCode, body)
		})

		t.Run("F8_dotfile", func(t *testing.T) {
			res, err := client.Get(endpoint + "/.hidden")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:          404,
				bodyNotContains: []string{"CUSTOM_404"},
			})
		})

		t.Run("F9_unicode_path", func(t *testing.T) {
			res, err := client.Get(endpoint + "/%E8%B7%AF%E5%BE%91/%E4%B8%AD%E6%96%87")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{status: 404, bodyContains: "CUSTOM_404"})
		})
	})

	t.Run("G_PathEdges", func(t *testing.T) {
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

		t.Run("G7_long_path", func(t *testing.T) {
			longPath := "/" + strings.Repeat("a", 4096)
			res, err := client.Get(endpoint + longPath)
			if err != nil {
				t.Logf("G7 long path error (acceptable): %v", err)
				return
			}
			defer res.Body.Close()
			t.Logf("G7 long path → status %d", res.StatusCode)
		})

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
				t.Skip("no ETag header returned")
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

		// G10: conditional request for MPA fallback route → 304 (cache revalidation)
		t.Run("G10_conditional_mpa_fallback", func(t *testing.T) {
			res1, err := client.Get(endpoint + "/projects")
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

			req, _ := http.NewRequest("GET", endpoint+"/projects", nil)
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
				t.Errorf("G10 conditional MPA fallback: want 304, got %d", res2.StatusCode)
			}
		})
	})

	t.Run("H_Methods", func(t *testing.T) {
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
				t.Errorf("HEAD body must be empty")
			}
		})

		// H3: HEAD /projects → 404
		t.Run("H3_head_mpa_route", func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), "HEAD", endpoint+"/projects", nil)
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
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
				t.Errorf("HEAD body must be empty")
			}
		})

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
				t.Errorf("HEAD body must be empty")
			}
		})
	})

	t.Run("I_Headers", func(t *testing.T) {
		t.Run("I4_root_headers", func(t *testing.T) {
			res, err := client.Get(endpoint + "/")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{status: 200, contentType: "text/html"})
			if res.Header.Get("Last-Modified") == "" && res.Header.Get("ETag") == "" {
				t.Error("want at least one of ETag or Last-Modified")
			}
		})

		t.Run("I8_asset_miss_content_type", func(t *testing.T) {
			res, err := client.Get(endpoint + "/assets/missing.js")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{status: 404, contentTypeNot: "text/html"})
		})

		// I10: MPA missing route → 404 text/html
		t.Run("I10_mpa_route_content_type", func(t *testing.T) {
			res, err := client.Get(endpoint + "/projects")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:      404,
				contentType: "text/html",
			})
		})
	})

	t.Run("Regression", func(t *testing.T) {
		// J2: MPA missing route must return 404, not 200
		t.Run("J2_mpa_missing_returns_404_not_200", func(t *testing.T) {
			res, err := client.Get(endpoint + "/projects")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:       404,
				bodyContains: "CUSTOM_404",
			})
		})

		// J4: MPA missing asset → plain 404, not CUSTOM_404
		t.Run("J4_mpa_missing_asset_returns_plain_404", func(t *testing.T) {
			res, err := client.Get(endpoint + "/assets/missing.js")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			assertResponse(t, res, expectation{
				status:          404,
				bodyContains:    "Not Found",
				bodyNotContains: []string{"CUSTOM_404"},
				contentTypeNot:  "text/html",
			})
		})
	})
}
