// Package update implements verified, staged updates. It never streams into a live executable.
package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const API = "https://api.github.com/repos/shini74744/Dui/releases"
const Downloads = "https://github.com/shini74744/Dui/releases/download/"

var panelTag = regexp.MustCompile("^v[0-9]+\\.[0-9]+\\.[0-9]+$")
var coreTag = regexp.MustCompile("^vx-[0-9]+\\.[0-9]+$")

type Asset struct {
	Name  string
	Size  int64
	State string
}
type Release struct {
	Tag        string `json:"tag_name"`
	Draft      bool
	Prerelease bool
	Assets     []Asset
}
type Offer struct {
	Version string `json:"version"`
	Asset   string `json:"asset"`
	Size    int64  `json:"size"`
}

func Newer(a, b string) bool {
	parse := func(s string) []int {
		s = strings.TrimPrefix(strings.TrimPrefix(s, "vx-"), "v")
		parts := strings.Split(s, ".")
		result := []int{}
		for _, p := range parts {
			n, e := strconv.Atoi(p)
			if e != nil {
				return nil
			}
			result = append(result, n)
		}
		return result
	}
	x, y := parse(a), parse(b)
	if len(x) == 0 || len(y) == 0 {
		return false
	}
	for i := 0; i < len(x) || i < len(y); i++ {
		aa, bb := 0, 0
		if i < len(x) {
			aa = x[i]
		}
		if i < len(y) {
			bb = y[i]
		}
		if aa != bb {
			return aa > bb
		}
	}
	return false
}
func AssetName(kind string) (string, error) {
	arch := runtime.GOARCH
	if arch == "arm" {
		arm := "5"
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "GOARM" {
					arm = strings.Split(s.Value, ",")[0]
				}
			}
		}
		arch = "armv" + arm
	}
	if runtime.GOOS != "linux" {
		return "", errors.New("unsupported_platform")
	}
	if kind == "panel" {
		switch arch {
		case "amd64", "386", "arm64", "armv5", "armv6", "armv7", "s390x":
			return "x-ui-linux-" + arch + ".tar.gz", nil
		}
	} else if kind == "core" {
		names := map[string]string{"amd64": "64", "386": "32", "arm64": "arm64-v8a", "armv5": "arm32-v5", "armv6": "arm32-v6", "armv7": "arm32-v7a", "s390x": "s390x", "riscv64": "riscv64", "loong64": "loong64", "ppc64le": "ppc64le", "mips64": "mips64", "mips64le": "mips64le"}
		if n := names[arch]; n != "" {
			return "Xray-linux-" + n + ".zip", nil
		}
	}
	return "", errors.New("unsupported_platform")
}
func (r Release) Offer(kind, asset string) (Offer, error) {
	valid := panelTag.MatchString(r.Tag)
	if kind == "core" {
		valid = coreTag.MatchString(r.Tag)
	}
	if !valid || r.Draft || r.Prerelease {
		return Offer{}, errors.New("invalid_release")
	}
	found := map[string]int64{}
	for _, a := range r.Assets {
		if a.State == "uploaded" && a.Size > 0 {
			found[a.Name] = a.Size
		}
	}
	if found[asset] == 0 || found[asset+".sha256"] == 0 || found[asset] > 1<<30 {
		return Offer{}, errors.New("incomplete_release")
	}
	return Offer{r.Tag, asset, found[asset]}, nil
}

// No total download timeout: slow but progressing transfers may continue.
// A connection stalled for 90 seconds fails without touching installed files.
type idleConn struct{ net.Conn }

func (c idleConn) Read(p []byte) (int, error) {
	_ = c.SetReadDeadline(time.Now().Add(90 * time.Second))
	return c.Conn.Read(p)
}
func Client() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		c, e := (&net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, addr)
		if e != nil {
			return nil, e
		}
		return idleConn{c}, nil
	}
	tr.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Transport: tr, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || r.URL.Scheme != "https" {
			return errors.New("unsafe_redirect")
		}
		return nil
	}}
}
func get(ctx context.Context, c *http.Client, url string) (*http.Response, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "DUI-verified-updater")
	resp, e := c.Do(req)
	if e != nil {
		return nil, errors.New("network_error")
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("http_%d", resp.StatusCode)
	}
	return resp, nil
}
func FetchJSON(ctx context.Context, c *http.Client, url string, dst any) error {
	r, e := get(ctx, c, url)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(dst)
}
func Latest(ctx context.Context, c *http.Client) (map[string]Offer, error) {
	offers := map[string]Offer{}
	for page := 1; page <= 10; page++ {
		var rs []Release
		if e := FetchJSON(ctx, c, fmt.Sprintf("%s?per_page=100&page=%d", API, page), &rs); e != nil {
			return nil, e
		}
		for _, r := range rs {
			for _, kind := range []string{"panel", "core"} {
				name, e := AssetName(kind)
				if e != nil {
					continue
				}
				o, e := r.Offer(kind, name)
				if e == nil && (offers[kind].Version == "" || Newer(o.Version, offers[kind].Version)) {
					offers[kind] = o
				}
			}
		}
		if len(rs) < 100 {
			return offers, nil
		}
	}
	return nil, errors.New("release_page_limit")
}
func Resolve(ctx context.Context, c *http.Client, kind, version string) (Offer, error) {
	if (kind == "panel" && !panelTag.MatchString(version)) || (kind == "core" && !coreTag.MatchString(version)) || (kind != "panel" && kind != "core") {
		return Offer{}, errors.New("invalid_release")
	}
	name, e := AssetName(kind)
	if e != nil {
		return Offer{}, e
	}
	var r Release
	if e = FetchJSON(ctx, c, API+"/tags/"+version, &r); e != nil {
		return Offer{}, e
	}
	if r.Tag != version {
		return Offer{}, errors.New("invalid_release")
	}
	return r.Offer(kind, name)
}

type Job struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Version    string    `json:"version"`
	Phase      string    `json:"phase"`
	Downloaded int64     `json:"downloaded"`
	Total      int64     `json:"total"`
	Error      string    `json:"error"`
	UpdatedAt  time.Time `json:"updatedAt"`
	StartedAt  time.Time `json:"startedAt"`
	// Internal metadata is never returned by the HTTP API.
	Root, Dir, Target, Panel, Core, DB, Config, WorkDir, Unit, Worker string
	Offer                                                             Offer
	OldHash, NewHash                                                  string
	CoreRunning                                                       bool
	PreviousCoreRunning                                               *bool
	ConfigAbsent                                                      bool
	ValidationConfig                                                  json.RawMessage
	Applied                                                           bool
	BackupReady                                                       bool
}

func (j Job) Busy() bool {
	switch j.Phase {
	case "queued", "connecting", "downloading", "verifying", "preparing", "installing", "restarting", "rolling_back":
		return true
	}
	return false
}
func (j Job) Public() map[string]any {
	return map[string]any{"id": j.ID, "kind": j.Kind, "version": j.Version, "phase": j.Phase, "downloaded": j.Downloaded, "total": j.Total, "error": j.Error, "updatedAt": j.UpdatedAt, "startedAt": j.StartedAt, "busy": j.Busy()}
}
func WriteJSON(path string, v any) error {
	data, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".state-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	return syncDir(filepath.Dir(path))
}
func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func Load(root string) (Job, error) {
	var j Job
	b, e := os.ReadFile(filepath.Join(root, "job.json"))
	if e != nil {
		return j, e
	}
	e = json.Unmarshal(b, &j)
	return j, e
}
func (j *Job) Save() error {
	j.UpdatedAt = time.Now().UTC()
	return WriteJSON(filepath.Join(j.Root, "job.json"), j)
}
func (j *Job) phase(p string) error { j.Phase = p; return j.Save() }
func Hash(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func Copy(src, dst string, mode os.FileMode) error {
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	if e == nil {
		e = out.Sync()
	}
	ce := out.Close()
	if e == nil {
		e = ce
	}
	return e
}
func Download(ctx context.Context, c *http.Client, j *Job) error {
	o := j.Offer
	url := Downloads + o.Version + "/" + o.Asset
	r, e := get(ctx, c, url+".sha256")
	if e != nil {
		return e
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 4097))
	r.Body.Close()
	if e != nil {
		return errors.New("checksum_unavailable")
	}
	fields := strings.Fields(string(b))
	if len(b) > 4096 || len(fields) != 2 || fields[1] != o.Asset || len(fields[0]) != 64 {
		return errors.New("invalid_checksum")
	}
	if _, e = hex.DecodeString(fields[0]); e != nil {
		return errors.New("invalid_checksum")
	}
	r, e = get(ctx, c, url)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.ContentLength >= 0 && r.ContentLength != o.Size {
		return errors.New("size_mismatch")
	}
	path := filepath.Join(j.Dir, "download.part")
	f, e := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return errors.New("disk_write_failed")
	}
	defer f.Close()
	j.Downloaded = 0
	j.Total = o.Size
	if e = j.phase("downloading"); e != nil {
		return e
	}
	h := sha256.New()
	buf := make([]byte, 128<<10)
	last := time.Now()
	for {
		n, re := r.Body.Read(buf)
		if n > 0 {
			if j.Downloaded+int64(n) > o.Size {
				return errors.New("size_mismatch")
			}
			if _, e = f.Write(buf[:n]); e != nil {
				return errors.New("disk_write_failed")
			}
			h.Write(buf[:n])
			j.Downloaded += int64(n)
			if time.Since(last) > time.Second {
				if e = j.Save(); e != nil {
					return e
				}
				last = time.Now()
			}
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			return errors.New("download_interrupted")
		}
	}
	if j.Downloaded != o.Size {
		return errors.New("size_mismatch")
	}
	if e = j.phase("verifying"); e != nil {
		return e
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), fields[0]) {
		return errors.New("checksum_mismatch")
	}
	if e = f.Sync(); e != nil {
		return errors.New("disk_write_failed")
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(path, filepath.Join(j.Dir, "package"))
}

// Extract only the exact executable. Reject traversal, links and duplicate entries;
// read the entire archive (including trailing gzip checksum) before accepting it.
func Extract(j *Job) error {
	archive := filepath.Join(j.Dir, "package")
	dst := filepath.Join(j.Dir, "new")
	found := false
	write := func(r io.Reader, n int64) error {
		if found || n <= 0 || n > 256<<20 {
			return errors.New("invalid_archive")
		}
		found = true
		f, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if e != nil {
			return e
		}
		_, e = io.CopyN(f, r, n)
		if e == nil {
			e = f.Sync()
		}
		ce := f.Close()
		if e == nil {
			e = ce
		}
		return e
	}
	safe := func(s string) bool {
		return !strings.HasPrefix(s, "/") && !strings.Contains(s, "\\") && !strings.Contains("/"+s+"/", "/../")
	}
	if j.Kind == "core" {
		z, e := zip.OpenReader(archive)
		if e != nil {
			return errors.New("invalid_archive")
		}
		defer z.Close()
		for _, f := range z.File {
			if !safe(f.Name) || f.Mode()&os.ModeSymlink != 0 {
				return errors.New("invalid_archive")
			}
			if f.Name != "xray" {
				continue
			}
			r, e := f.Open()
			if e != nil {
				return e
			}
			e = write(r, int64(f.UncompressedSize64))
			if e == nil {
				_, e = io.Copy(io.Discard, r)
			}
			r.Close()
			if e != nil {
				return errors.New("invalid_archive")
			}
		}
	} else {
		f, e := os.Open(archive)
		if e != nil {
			return e
		}
		defer f.Close()
		gz, e := gzip.NewReader(f)
		if e != nil {
			return errors.New("invalid_archive")
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		var total int64
		for {
			h, e := tr.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return errors.New("invalid_archive")
			}
			total += h.Size
			if total > 2<<30 || !safe(h.Name) || (h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir) {
				return errors.New("invalid_archive")
			}
			if h.Name == "x-ui/x-ui" {
				if e = write(tr, h.Size); e != nil {
					return e
				}
			}
		}
		if _, e = io.Copy(io.Discard, io.LimitReader(gz, 1<<20)); e != nil {
			return errors.New("invalid_archive")
		}
	}
	if !found {
		return errors.New("binary_missing")
	}
	return nil
}

var upstreamCoreVersion = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

func KnownVersion(kind, version string) bool {
	if kind == "panel" {
		return panelTag.MatchString(version)
	}
	return kind == "core" && (coreTag.MatchString(version) || upstreamCoreVersion.MatchString(version))
}

func BinaryVersion(path, kind string) (string, error) {
	return binaryVersion(path, kind, 15*time.Second)
}

// Keep dashboard requests bounded without shortening staged-binary verification.
func InstalledVersion(path, kind string) (string, error) {
	return binaryVersion(path, kind, 5*time.Second)
}

func binaryVersion(path, kind string, timeout time.Duration) (string, error) {
	arg := "-v"
	if kind == "core" {
		arg = "-version"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	b, e := exec.CommandContext(ctx, path, arg).Output()
	if e != nil {
		return "", errors.New("binary_unusable")
	}
	if kind == "panel" {
		v := "v" + strings.TrimPrefix(strings.TrimSpace(string(b)), "v")
		if !KnownVersion(kind, v) {
			return "", errors.New("binary_unusable")
		}
		return v, nil
	}
	f := strings.Fields(string(b))
	if len(f) < 2 || f[0] != "Xray" || !KnownVersion(kind, f[1]) {
		return "", errors.New("binary_unusable")
	}
	return f[1], nil
}
func ValidateBinary(j *Job) error {
	path := filepath.Join(j.Dir, "new")
	v, e := BinaryVersion(path, j.Kind)
	if e != nil {
		return e
	}
	if v != j.Version {
		return errors.New("version_mismatch")
	}
	if j.Kind == "core" {
		data, e := os.ReadFile(j.Config)
		if os.IsNotExist(e) && len(j.ValidationConfig) > 0 {
			data, e = j.ValidationConfig, nil
		}
		if e != nil || !json.Valid(data) {
			return errors.New("config_unavailable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, path, "run", "-test", "-format", "json", "-config", "stdin:")
		cmd.Dir = j.WorkDir
		cmd.Stdin = strings.NewReader(string(data))
		cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Dir(j.Core))
		if e = cmd.Run(); e != nil {
			return errors.New("config_rejected")
		}
	}
	j.NewHash, e = Hash(path)
	return e
}

func ValidVersion(kind, version string) bool {
	return (kind == "panel" && panelTag.MatchString(version)) || (kind == "core" && coreTag.MatchString(version))
}
