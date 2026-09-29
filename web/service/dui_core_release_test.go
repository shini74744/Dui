package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type coreRoundTrip func(*http.Request) (*http.Response, error)

func (f coreRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func coreResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func testCoreRelease(tag, asset string) duiCoreRelease {
	r := duiCoreRelease{TagName: tag}
	for _, name := range []string{asset, asset + ".sha256", "SHA256SUMS"} {
		r.Assets = append(r.Assets, struct {
			Name  string `json:"name"`
			State string `json:"state"`
			Size  int64  `json:"size"`
		}{name, "uploaded", 100})
	}
	return r
}

func TestDUICoreVersionsOnlyPublishedMatchingAssets(t *testing.T) {
	asset := "Xray-linux-64.zip"
	valid := testCoreRelease("vx-26.0", asset)
	draft := testCoreRelease("xray-v26.3.27-dui.2", asset)
	draft.Draft = true
	pre := testCoreRelease("xray-v26.3.27-dui.3", asset)
	pre.Prerelease = true
	partial := testCoreRelease("xray-v26.3.27-dui.4", asset)
	partial.Assets = partial.Assets[:1]
	uploading := testCoreRelease("xray-v26.3.27-dui.5", asset)
	uploading.Assets[0].State = "starter"
	wrongArch := testCoreRelease("xray-v26.3.27-dui.6", "Xray-linux-arm64-v8a.zip")
	pages := [][]duiCoreRelease{{testCoreRelease("v26.9.35", asset), testCoreRelease("v26.3.27", asset), draft, pre, partial, uploading, wrongArch}, {valid, valid}}
	calls := 0
	c := duiCoreClient{http: &http.Client{Transport: coreRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		want := fmt.Sprintf("%s?per_page=30&page=%d", duiCoreAPI, calls)
		if r.URL.String() != want || calls > len(pages) {
			t.Fatalf("unexpected URL %s", r.URL)
		}
		body, _ := json.Marshal(pages[calls-1])
		resp := coreResponse(200, string(body))
		if calls == 1 {
			resp.Header.Set("Link", `<https://untrusted.example/next>; rel="next"`)
		}
		return resp, nil
	})}}
	versions, err := c.versions(asset)
	if err != nil || len(versions) != 1 || versions[0] != valid.TagName || calls != 2 {
		t.Fatalf("versions=%v calls=%d err=%v", versions, calls, err)
	}
}

func TestDUICoreRejectsOfficialAndArbitraryTagsBeforeNetwork(t *testing.T) {
	c := duiCoreClient{http: &http.Client{Transport: coreRoundTrip(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request %s", r.URL)
		return nil, nil
	})}}
	for _, tag := range []string{"v26.3.27", "v26.9.35", "../../other", "vx-26.0/extra", "vx-26.0?source=official", "vx-26", "https://example.com/core", "xray-v26.3.27-dui.1/extra", ""} {
		if _, err := c.download(tag, "Xray-linux-64.zip"); err == nil {
			t.Errorf("accepted %q", tag)
		}
	}
	if err := (&ServerService{}).UpdateXray("v26.3.27"); err == nil {
		t.Fatal("service accepted official core")
	}
}

func TestDUICoreAssetARMVariants(t *testing.T) {
	for _, tc := range []struct{ arch, arm, want string }{
		{"amd64", "", "64"}, {"386", "", "32"}, {"arm64", "", "arm64-v8a"},
		{"arm", "5", "arm32-v5"}, {"arm", "6,hardfloat", "arm32-v6"}, {"arm", "7", "arm32-v7a"},
		{"arm", "", "arm32-v5"}, {"s390x", "", "s390x"},
	} {
		got, err := duiCoreAsset("linux", tc.arch, tc.arm)
		if err != nil || got != "Xray-linux-"+tc.want+".zip" {
			t.Fatalf("%+v: %s %v", tc, got, err)
		}
	}
	if _, err := duiCoreAsset("linux", "arm", "9"); err == nil {
		t.Fatal("accepted invalid GOARM")
	}
}

func TestDUICoreDownloadVerificationAndHTTPFailures(t *testing.T) {
	asset, tag := "Xray-linux-64.zip", "vx-26.0"
	payload := "test core archive"
	for _, tc := range []struct {
		name     string
		code     int
		checksum string
		complete bool
		ok       bool
	}{
		{"valid", 200, fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(payload)), asset), true, true},
		{"tampered", 200, strings.Repeat("0", 64) + "  " + asset, true, false},
		{"wrong filename", 200, strings.Repeat("0", 64) + "  other.zip", true, false},
		{"not found", 404, "", true, false},
		{"incomplete release", 200, "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			c := duiCoreClient{http: &http.Client{Transport: coreRoundTrip(func(r *http.Request) (*http.Response, error) {
				switch r.URL.String() {
				case duiCoreAPI + "/tags/" + tag:
					release := testCoreRelease(tag, asset)
					if !tc.complete {
						release.Assets = nil
					}
					body, _ := json.Marshal(release)
					return coreResponse(tc.code, string(body)), nil
				case duiCoreDownloads + tag + "/" + asset + ".sha256":
					return coreResponse(200, tc.checksum), nil
				case duiCoreDownloads + tag + "/" + asset:
					return coreResponse(200, payload), nil
				default:
					t.Fatalf("unexpected source %s", r.URL)
					return nil, nil
				}
			})}}
			path, err := c.download(tag, asset)
			if (err == nil) != tc.ok {
				t.Fatalf("path=%q err=%v", path, err)
			}
			if tc.ok {
				defer os.Remove(path)
				data, err := os.ReadFile(path)
				if err != nil || string(data) != payload {
					t.Fatalf("bad downloaded data: %v", err)
				}
			} else {
				if path != "" {
					t.Fatalf("failed download returned %s", path)
				}
				left, _ := os.ReadDir(os.TempDir())
				if len(left) != 0 {
					t.Fatal("failed download left temporary files")
				}
			}
		})
	}
}

func TestDUICoreVersionsReportsHTTPError(t *testing.T) {
	c := duiCoreClient{http: &http.Client{Transport: coreRoundTrip(func(*http.Request) (*http.Response, error) { return coreResponse(403, `{"message":"rate limit"}`), nil })}}
	if _, err := c.versions("Xray-linux-64.zip"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected HTTP error, got %v", err)
	}
}
