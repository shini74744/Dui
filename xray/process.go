package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	"x-ui/config"
	"x-ui/logger"
	"x-ui/util/common"
)

func GetBinaryName() string {
	return fmt.Sprintf("xray-%s-%s", runtime.GOOS, runtime.GOARCH)
}

func GetBinaryPath() string {
	return config.GetBinFolderPath() + "/" + GetBinaryName()
}

func GetConfigPath() string {
	return config.GetBinFolderPath() + "/config.json"
}

func GetGeositePath() string {
	return config.GetBinFolderPath() + "/geosite.dat"
}

func GetGeoipPath() string {
	return config.GetBinFolderPath() + "/geoip.dat"
}

func GetIPLimitLogPath() string {
	return config.GetLogFolder() + "/3xipl.log"
}

func GetIPLimitBannedLogPath() string {
	return config.GetLogFolder() + "/3xipl-banned.log"
}

func GetIPLimitBannedPrevLogPath() string {
	return config.GetLogFolder() + "/3xipl-banned.prev.log"
}

func GetAccessPersistentLogPath() string {
	return config.GetLogFolder() + "/3xipl-ap.log"
}

func GetAccessPersistentPrevLogPath() string {
	return config.GetLogFolder() + "/3xipl-ap.prev.log"
}

func GetAccessLogPath() (string, error) {
	config, err := os.ReadFile(GetConfigPath())
	if err != nil {
		logger.Warningf("Failed to read configuration file: %s", err)
		return "", err
	}

	jsonConfig := map[string]any{}
	err = json.Unmarshal([]byte(config), &jsonConfig)
	if err != nil {
		logger.Warningf("Failed to parse JSON configuration: %s", err)
		return "", err
	}

	if jsonConfig["log"] != nil {
		jsonLog := jsonConfig["log"].(map[string]any)
		if jsonLog["access"] != nil {
			accessLogPath := jsonLog["access"].(string)
			return accessLogPath, nil
		}
	}
	return "", err
}

func stopProcess(p *Process) {
	p.Stop()
}

type Process struct {
	*process
}

func NewProcess(xrayConfig *Config) *Process {
	p := &Process{newProcess(xrayConfig)}
	runtime.SetFinalizer(p, stopProcess)
	return p
}

type process struct {
	lifecycle sync.Mutex
	cmd       *exec.Cmd
	done      chan struct{}

	version string
	apiPort int

	onlineClients []string
	mutex         sync.RWMutex

	config    *Config
	logWriter *LogWriter
	exitErr   error
	startTime time.Time
}

func newProcess(config *Config) *process {
	return &process{
		version:   "Unknown",
		config:    config,
		logWriter: NewLogWriter(),
		startTime: time.Now(),
	}
}

func (p *process) IsRunning() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if p.done == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func (p *process) GetErr() error {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.exitErr
}

func (p *process) GetResult() string {
	line, err := p.logWriter.LastLine(), p.GetErr()
	if len(line) == 0 && err != nil {
		return err.Error()
	}
	return line
}

func (p *process) GetVersion() string {
	return p.version
}

func (p *Process) GetAPIPort() int {
	return p.apiPort
}

func (p *Process) GetConfig() *Config {
	return p.config
}

func (p *Process) GetOnlineClients() []string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	clientsCopy := make([]string, len(p.onlineClients))
	copy(clientsCopy, p.onlineClients)
	return clientsCopy
}

func (p *Process) SetOnlineClients(clients []string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.onlineClients = clients
}

func (p *Process) GetUptime() uint64 {
	return uint64(time.Since(p.startTime).Seconds())
}

func (p *process) refreshAPIPort() {
	for _, inbound := range p.config.InboundConfigs {
		if inbound.Tag == "api" {
			p.apiPort = inbound.Port
			break
		}
	}
}

func (p *process) refreshVersion() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, GetBinaryPath(), "-version")
	data, err := cmd.Output()
	if err != nil {
		p.version = "Unknown"
	} else {
		datas := bytes.Split(data, []byte(" "))
		if len(datas) <= 1 {
			p.version = "Unknown"
		} else {
			p.version = string(datas[1])
		}
	}
}

func (p *process) Start() (err error) {
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	if p.IsRunning() {
		return errors.New("xray is already running")
	}

	defer func() {
		if err != nil {
			logger.Error("Failure in running xray-core process: ", err)
			p.mutex.Lock()
			p.exitErr = err
			p.mutex.Unlock()
		}
	}()

	if err := ValidateStrategyObservatorySupport(p.config); err != nil {
		return err
	}
	if err := ValidateCloseWaitSupport(p.config); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p.config, "", "  ")
	if err != nil {
		return common.NewErrorf("Failed to generate XRAY configuration files: %v", err)
	}

	err = os.MkdirAll(config.GetLogFolder(), 0o770)
	if err != nil {
		logger.Warningf("Failed to create log folder: %s", err)
	}

	configPath := GetConfigPath()
	err = os.WriteFile(configPath, data, fs.FileMode(0600))
	if err != nil {
		return common.NewErrorf("Failed to write configuration file: %v", err)
	}

	cmd := exec.Command(GetBinaryPath(), "-c", configPath)
	cmd.Stdout = p.logWriter
	cmd.Stderr = p.logWriter
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	p.mutex.Lock()
	p.cmd = cmd
	p.done = done
	p.exitErr = nil
	p.mutex.Unlock()

	go func() {
		err := cmd.Wait()
		if err != nil {
			logger.Error("Failure in running xray-core:", err)
			p.mutex.Lock()
			p.exitErr = err
			p.mutex.Unlock()
		}
		close(done)
	}()

	p.refreshVersion()
	p.refreshAPIPort()
	select {
	case <-done:
		if err := p.GetErr(); err != nil {
			return err
		}
		return errors.New("xray exited during startup")
	case <-time.After(300 * time.Millisecond):
		return nil
	}
}

func (p *process) Stop() error {
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	p.mutex.RLock()
	cmd, done := p.cmd, p.done
	p.mutex.RUnlock()
	if cmd == nil || done == nil {
		return errors.New("xray is not running")
	}
	select {
	case <-done:
		return nil
	default:
	}
	var err error
	if runtime.GOOS == "windows" {
		err = cmd.Process.Kill()
	} else {
		err = cmd.Process.Signal(syscall.SIGTERM)
	}
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		select {
		case <-done:
			return nil
		case <-time.After(5 * time.Second):
			return errors.New("xray did not stop")
		}
	}
}

func writeCrashReport(m []byte) error {
	crashReportPath := config.GetBinFolderPath() + "/core_crash_" + time.Now().Format("20060102_150405") + ".log"
	return os.WriteFile(crashReportPath, m, os.ModePerm)
}
