package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

const duiCoreAPI = "https://api.github.com/repos/shini74744/Dui/releases"
const duiCoreDownloads = "https://github.com/shini74744/Dui/releases/download/"

var duiCoreTag = regexp.MustCompile(`^(vx-[0-9]+\.[0-9]+|xray-v[0-9]+\.[0-9]+\.[0-9]+-dui\.[1-9][0-9]*)$`)

type duiCoreRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name  string `json:"name"`
		State string `json:"state"`
		Size  int64  `json:"size"`
	} `json:"assets"`
}

func (r duiCoreRelease) supports(asset string) bool {
	if r.Draft || r.Prerelease || !duiCoreTag.MatchString(r.TagName) {
		return false
	}
	found := map[string]bool{}
	for _, a := range r.Assets {
		if a.State == "uploaded" && a.Size > 0 {
			found[a.Name] = true
		}
	}
	return found[asset] && found[asset+".sha256"] && found["SHA256SUMS"]
}

// GOARCH is arm for all 32-bit ARM builds. Preserve the build's GOARM variant.
func duiCoreAsset(goos, arch, arm string) (string, error) {
	if goos == "darwin" {
		goos = "macos"
	}
	switch goos {
	case "linux", "windows", "macos", "freebsd", "openbsd", "android":
	default:
		return "", fmt.Errorf("unsupported Xray operating system: %s", goos)
	}
	switch arch {
	case "amd64":
		if goos == "android" {
			arch = "amd64"
		} else {
			arch = "64"
		}
	case "386":
		arch = "32"
	case "arm64":
		arch = "arm64-v8a"
	case "arm":
		arm = strings.Split(arm, ",")[0]
		switch arm {
		case "7":
			arch = "arm32-v7a"
		case "6":
			arch = "arm32-v6"
		case "5", "":
			arch = "arm32-v5"
		default:
			return "", fmt.Errorf("unsupported Xray ARM variant: %s", arm)
		}
	case "s390x", "riscv64", "loong64", "ppc64", "ppc64le", "mips64", "mips64le":
	default:
		return "", fmt.Errorf("unsupported Xray architecture: %s", arch)
	}
	return fmt.Sprintf("Xray-%s-%s.zip", goos, arch), nil
}

func currentDUICoreAsset() (string, error) {
	arm := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "GOARM" {
				arm = setting.Value
				break
			}
		}
	}
	return duiCoreAsset(runtime.GOOS, runtime.GOARCH, arm)
}

type duiCoreClient struct{ http *http.Client }

func newDUICoreClient() duiCoreClient {
	return duiCoreClient{http: &http.Client{Timeout: 5 * time.Minute}}
}

func (c duiCoreClient) get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "DUI-core-updater")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("DUI core download returned HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func (c duiCoreClient) json(url string, target any) (string, error) {
	resp, err := c.get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	err = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(target)
	return resp.Header.Get("Link"), err
}

func (c duiCoreClient) versions(asset string) ([]string, error) {
	versions := make([]string, 0)
	seen := map[string]bool{}
	for page := 1; page <= 20; page++ {
		var releases []duiCoreRelease
		link, err := c.json(fmt.Sprintf("%s?per_page=30&page=%d", duiCoreAPI, page), &releases)
		if err != nil {
			return nil, err
		}
		for _, release := range releases {
			if release.supports(asset) && !seen[release.TagName] {
				versions = append(versions, release.TagName)
				seen[release.TagName] = true
			}
		}
		if !strings.Contains(link, `rel="next"`) {
			return versions, nil
		}
	}
	return nil, fmt.Errorf("DUI core release pagination limit reached")
}

func (c duiCoreClient) download(version, asset string) (string, error) {
	if !duiCoreTag.MatchString(version) {
		return "", fmt.Errorf("only DUI Xray core releases can be installed")
	}
	var release duiCoreRelease
	if _, err := c.json(duiCoreAPI+"/tags/"+version, &release); err != nil {
		return "", err
	}
	if release.TagName != version || !release.supports(asset) {
		return "", fmt.Errorf("DUI core release has no complete package for this platform")
	}
	url := duiCoreDownloads + version + "/" + asset
	resp, err := c.get(url + ".sha256")
	if err != nil {
		return "", err
	}
	checksum, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	resp.Body.Close()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(checksum))
	if len(checksum) > 4096 || len(fields) != 2 || fields[1] != asset || len(fields[0]) != 64 {
		return "", fmt.Errorf("invalid DUI core checksum file")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", fmt.Errorf("invalid DUI core SHA-256")
	}
	resp, err = c.get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	file, err := os.CreateTemp("", "dui-xray-*.zip")
	if err != nil {
		return "", err
	}
	keep := false
	defer func() {
		file.Close()
		if !keep {
			os.Remove(file.Name())
		}
	}()
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, hash), resp.Body); err != nil {
		return "", err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), fields[0]) {
		return "", fmt.Errorf("DUI core SHA-256 verification failed")
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	keep = true
	return file.Name(), nil
}
