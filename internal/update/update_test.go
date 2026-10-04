package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func rewriteClient(url string) *http.Client {
	return &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme = "http"
		r.URL.Host = strings.TrimPrefix(url, "http://")
		return http.DefaultTransport.RoundTrip(r)
	})}
}
func job(t *testing.T, kind string) *Job {
	t.Helper()
	root := t.TempDir()
	target := filepath.Join(root, "installed")
	os.WriteFile(target, []byte("original"), 0755)
	return &Job{ID: "test", Kind: kind, Version: "vx-26.7", Root: root, Dir: root, Target: target, Offer: Offer{Version: "vx-26.7", Asset: "Xray-linux-64.zip"}}
}
func TestDownloadOnlyVerifiedCompletion(t *testing.T) {
	data := bytes.Repeat([]byte("download-data"), 5000)
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	for _, mode := range []string{"complete", "short", "long", "checksum", "http_error", "slow"} {
		t.Run(mode, func(t *testing.T) {
			j := job(t, "core")
			j.Offer.Size = int64(len(data))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, ".sha256") {
					s := sum
					if mode == "checksum" {
						s = strings.Repeat("0", 64)
					}
					fmt.Fprintf(w, "%s  %s\n", s, j.Offer.Asset)
					return
				}
				if mode == "http_error" {
					w.WriteHeader(503)
					return
				}
				b := data
				if mode == "short" {
					b = b[:len(b)-1]
				}
				if mode == "long" {
					b = append(append([]byte{}, b...), 1)
				}
				if mode == "slow" {
					for off := 0; off < len(b); off += 10000 {
						end := off + 10000
						if end > len(b) {
							end = len(b)
						}
						w.Write(b[off:end])
						w.(http.Flusher).Flush()
						time.Sleep(210 * time.Millisecond)
					}
					return
				}
				w.Write(b)
			}))
			defer server.Close()
			e := Download(context.Background(), rewriteClient(server.URL), j)
			ok := mode == "complete" || mode == "slow"
			if (e == nil) != ok {
				t.Fatalf("unexpected result: %v", e)
			}
			original, _ := os.ReadFile(j.Target)
			if string(original) != "original" {
				t.Fatal("live executable changed during download")
			}
			_, stat := os.Stat(filepath.Join(j.Dir, "package"))
			if (stat == nil) != ok {
				t.Fatal("unverified package accepted")
			}
			if ok && j.Downloaded != int64(len(data)) {
				t.Fatal("progress incomplete")
			}
			if mode == "slow" {
				saved, e := Load(j.Root)
				if e != nil || saved.Downloaded != int64(len(data)) || saved.Phase != "verifying" {
					t.Fatal("progress not persisted")
				}
			}
		})
	}
}
func TestCancelledDownloadLeavesLiveBinary(t *testing.T) {
	j := job(t, "core")
	j.Offer.Size = 100
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			fmt.Fprintf(w, "%s  %s", strings.Repeat("0", 64), j.Offer.Asset)
			return
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if e := Download(ctx, rewriteClient(server.URL), j); e == nil {
		t.Fatal("cancelled download accepted")
	}
	b, _ := os.ReadFile(j.Target)
	if string(b) != "original" {
		t.Fatal("target changed")
	}
}
func zipBytes(entries []string) []byte {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, n := range entries {
		w, _ := z.Create(n)
		w.Write([]byte("binary"))
	}
	z.Close()
	return b.Bytes()
}
func tarBytes(name string, typ byte) []byte {
	var b bytes.Buffer
	g := gzip.NewWriter(&b)
	tr := tar.NewWriter(g)
	h := &tar.Header{Name: name, Mode: 0755, Typeflag: typ}
	if typ == tar.TypeReg {
		h.Size = 6
	}
	tr.WriteHeader(h)
	if typ == tar.TypeReg {
		tr.Write([]byte("binary"))
	}
	tr.Close()
	g.Close()
	return b.Bytes()
}
func TestArchiveValidation(t *testing.T) {
	cases := []struct {
		name, kind string
		data       []byte
		ok         bool
	}{
		{"zip", "core", zipBytes([]string{"xray"}), true}, {"zip_missing", "core", zipBytes([]string{"nope"}), false},
		{"zip_duplicate", "core", zipBytes([]string{"xray", "xray"}), false}, {"zip_traversal", "core", zipBytes([]string{"../xray"}), false},
		{"tar", "panel", tarBytes("x-ui/x-ui", tar.TypeReg), true}, {"tar_link", "panel", tarBytes("x-ui/x-ui", tar.TypeSymlink), false},
		{"tar_traversal", "panel", tarBytes("../x-ui/x-ui", tar.TypeReg), false}, {"truncated", "panel", []byte("bad"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := job(t, c.kind)
			os.WriteFile(filepath.Join(j.Dir, "package"), c.data, 0600)
			if e := Extract(j); (e == nil) != c.ok {
				t.Fatalf("%v", e)
			}
			b, _ := os.ReadFile(j.Target)
			if string(b) != "original" {
				t.Fatal("target overwritten")
			}
		})
	}
}
func TestVersionsAndReleaseCompleteness(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"v26.9.47", "v26.9.46", true}, {"v26.10.1", "26.9.47", true}, {"vx-26.10", "vx-26.9", true}, {"v26.9.46", "v26.9.47", false}, {"vx-26.6", "vx-26.6", false}, {"bad", "26.6", false}} {
		if Newer(c.a, c.b) != c.want {
			t.Fatal(c)
		}
	}
	r := Release{Tag: "v26.9.47", Assets: []Asset{{"x.tar.gz", 100, "uploaded"}, {"x.tar.gz.sha256", 80, "uploaded"}}}
	if _, e := r.Offer("panel", "x.tar.gz"); e != nil {
		t.Fatal(e)
	}
	r.Draft = true
	if _, e := r.Offer("panel", "x.tar.gz"); e == nil {
		t.Fatal("draft")
	}
	r.Draft = false
	r.Assets = r.Assets[:1]
	if _, e := r.Offer("panel", "x.tar.gz"); e == nil {
		t.Fatal("missing checksum")
	}
	for _, s := range []string{"v26.9.47/../../x", "https://bad", "vx-26.6;reboot", "26.9.47"} {
		if ValidVersion("panel", s) || ValidVersion("core", s) {
			t.Fatal("invalid tag accepted")
		}
	}
}
func TestStateAtomicAndPrivate(t *testing.T) {
	j := job(t, "core")
	j.Downloaded = 123
	j.Total = 999
	j.Phase = "downloading"
	if e := j.Save(); e != nil {
		t.Fatal(e)
	}
	got, e := Load(j.Root)
	if e != nil || got.Downloaded != 123 || !got.Busy() {
		t.Fatal(got, e)
	}
	st, _ := os.Stat(filepath.Join(j.Root, "job.json"))
	if st.Mode().Perm() != 0600 {
		t.Fatal("public permissions")
	}
	if _, ok := got.Public()["Target"]; ok {
		t.Fatal("internal paths leaked")
	}
}

var _ io.Reader
