package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"x-ui/config"
	"x-ui/internal/update"
	"x-ui/xray"
)

var releaseCache struct {
	sync.Mutex
	offers    map[string]update.Offer
	checked   time.Time
	attempted time.Time
	checking  bool
	err       string
}

func updatesRoot() string { return filepath.Join(config.GetDBFolderPath(), "updates") }
func updateItem(kind, current string, offer update.Offer) map[string]any {
	known := update.KnownVersion(kind, current)
	available := offer.Version != "" && (!known || update.Newer(offer.Version, current) || (kind == "core" && !strings.HasPrefix(current, "vx-")))
	return map[string]any{
		"current": current, "currentKnown": known, "latest": offer.Version,
		"available": available,
		"upToDate":  known && offer.Version != "" && !available,
		"size":      offer.Size,
	}
}

func (s *ServerService) UpdateStatus(refresh bool) map[string]any {
	// Installed version must remain readable even when configuration prevents startup.
	coreVersion, _ := update.InstalledVersion(xray.GetBinaryPath(), "core")
	releaseCache.Lock()
	if !releaseCache.checking && (releaseCache.attempted.IsZero() || time.Since(releaseCache.attempted) > time.Hour || (refresh && time.Since(releaseCache.attempted) > time.Minute)) {
		releaseCache.checking = true
		releaseCache.attempted = time.Now()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			offers, e := update.Latest(ctx, update.Client())
			releaseCache.Lock()
			defer releaseCache.Unlock()
			releaseCache.checking = false
			if e != nil {
				releaseCache.err = e.Error()
				return
			}
			releaseCache.offers = offers
			releaseCache.checked = time.Now().UTC()
			releaseCache.err = ""
		}()
	}
	result := map[string]any{"checking": releaseCache.checking, "checkedAt": releaseCache.checked, "checkError": releaseCache.err}
	items := map[string]any{}
	for _, kind := range []string{"panel", "core"} {
		current := "v" + config.GetVersion()
		if kind == "core" {
			current = coreVersion
		}
		o := releaseCache.offers[kind]
		items[kind] = updateItem(kind, current, o)
	}
	releaseCache.Unlock()
	result["items"] = items
	exe, _ := os.Executable()
	result["supported"] = update.Supported(exe)
	if j, e := update.Status(updatesRoot()); e == nil {
		result["job"] = j.Public()
	}
	return result
}
func (s *ServerService) StartUpdate(kind, version string) (map[string]any, error) {
	if !update.ValidVersion(kind, version) {
		return nil, errors.New("invalid_release")
	}
	if kind == "panel" && !update.Newer(version, "v"+config.GetVersion()) {
		return nil, errors.New("already_current")
	}
	if kind == "core" {
		if installed, e := update.InstalledVersion(xray.GetBinaryPath(), "core"); e == nil && version == installed {
			return nil, errors.New("already_current")
		}
		template, e := (&SettingService{}).GetXrayConfigTemplate()
		if e != nil {
			return nil, e
		}
		var cfg xray.Config
		if e = json.Unmarshal([]byte(template), &cfg); e != nil {
			return nil, e
		}
		if e = xray.ValidateStrategyObservatoryTarget(&cfg, version); e != nil {
			return nil, e
		}
		if e = xray.ValidateCloseWaitTarget(&cfg, version); e != nil {
			return nil, e
		}
	}
	panel, e := os.Executable()
	if e != nil {
		return nil, e
	}
	core, e := filepath.Abs(xray.GetBinaryPath())
	if e != nil {
		return nil, e
	}
	cfg, e := filepath.Abs(xray.GetConfigPath())
	if e != nil {
		return nil, e
	}
	db, e := filepath.Abs(config.GetDBPath())
	if e != nil {
		return nil, e
	}
	wd, e := os.Getwd()
	if e != nil {
		return nil, e
	}
	target := panel
	if kind == "core" {
		target = core
	}
	// Root-owned regular files only; replacing a symlink would break custom layouts.
	for _, p := range []string{target, panel, core, db} {
		st, e := os.Lstat(p)
		if e != nil || !st.Mode().IsRegular() {
			return nil, errors.New("unsupported_installation")
		}
	}
	var validationConfig json.RawMessage
	if st, err := os.Lstat(cfg); err == nil {
		if !st.Mode().IsRegular() {
			return nil, errors.New("unsupported_installation")
		}
	} else if !os.IsNotExist(err) {
		return nil, errors.New("config_unavailable")
	} else if kind == "core" {
		// Stage a complete configuration privately; never create or overwrite the live file here.
		generated, err := s.xrayService.buildXrayConfig(false)
		if err != nil {
			return nil, errors.New("config_unavailable")
		}
		validationConfig, err = json.Marshal(generated)
		if err != nil {
			return nil, errors.New("config_unavailable")
		}
	}
	wasRunning := s.xrayService.IsXrayRunning()
	j, e := update.Start(update.Job{Root: updatesRoot(), Kind: kind, Version: version, Target: target, Panel: panel, Core: core, Config: cfg, DB: db, WorkDir: wd, CoreRunning: wasRunning || kind == "core", PreviousCoreRunning: &wasRunning, ValidationConfig: validationConfig})
	if e != nil {
		return nil, e
	}
	return j.Public(), nil
}
