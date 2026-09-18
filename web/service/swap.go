package service

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultSwapFile     = "/swapfile"
	duiSwapSysctl       = "/etc/sysctl.d/99-dui-swap.conf"
	duiSwapFstab        = "/etc/fstab"
	maxManagedSwapMB    = int64(262144)
	minFreeReserveBytes = uint64(64 * 1024 * 1024)
)

type SwapSource struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     uint64 `json:"size"`
	Used     uint64 `json:"used"`
	Priority int    `json:"priority"`
	Managed  bool   `json:"managed"`
}

type SwapManageStatus struct {
	Total         uint64       `json:"total"`
	Used          uint64       `json:"used"`
	Swappiness    int          `json:"swappiness"`
	ManagedPath   string       `json:"managedPath"`
	ManagedSize   uint64       `json:"managedSize"`
	ManagedActive bool         `json:"managedActive"`
	Sources       []SwapSource `json:"sources"`
	Note          string       `json:"note"`
}

type SwapApplyOptions struct {
	TotalMB    int64
	Swappiness int
}

type SwapService struct{}

func readProcSwaps() ([]SwapSource, error) {
	f, err := os.Open("/proc/swaps")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var result []SwapSource
	scanner := bufio.NewScanner(f)
	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 {
			continue
		}
		sizeKB, err1 := strconv.ParseUint(fields[2], 10, 64)
		usedKB, err2 := strconv.ParseUint(fields[3], 10, 64)
		prio, err3 := strconv.Atoi(fields[4])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		result = append(result, SwapSource{
			Name:     fields[0],
			Type:     fields[1],
			Size:     sizeKB * 1024,
			Used:     usedKB * 1024,
			Priority: prio,
		})
	}
	return result, scanner.Err()
}

func regularSwapFile(path string) bool {
	if path == "" || !filepath.IsAbs(path) || strings.HasPrefix(filepath.Clean(path), "/dev/") {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func chooseManagedSwapPath(sources []SwapSource) string {
	for _, src := range sources {
		if filepath.Clean(src.Name) == defaultSwapFile && src.Type == "file" {
			return defaultSwapFile
		}
	}
	if regularSwapFile(defaultSwapFile) {
		return defaultSwapFile
	}

	var fileCandidates []string
	for _, src := range sources {
		if src.Type == "file" && regularSwapFile(src.Name) {
			fileCandidates = append(fileCandidates, filepath.Clean(src.Name))
		}
	}
	if len(fileCandidates) == 1 {
		return fileCandidates[0]
	}
	return defaultSwapFile
}

func readSwappiness() int {
	data, err := os.ReadFile("/proc/sys/vm/swappiness")
	if err != nil {
		return 60
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || n < 0 || n > 100 {
		return 60
	}
	return n
}

func (s *SwapService) Status() (*SwapManageStatus, error) {
	sources, err := readProcSwaps()
	if err != nil {
		return nil, err
	}
	managedPath := chooseManagedSwapPath(sources)
	st := &SwapManageStatus{
		Swappiness:  readSwappiness(),
		ManagedPath: managedPath,
		Sources:     sources,
	}

	var external uint64
	for i := range st.Sources {
		src := &st.Sources[i]
		src.Managed = filepath.Clean(src.Name) == filepath.Clean(managedPath) && src.Type == "file"
		st.Total += src.Size
		st.Used += src.Used
		if src.Managed {
			st.ManagedSize = src.Size
			st.ManagedActive = true
		} else {
			external += src.Size
		}
	}
	if info, err := os.Stat(managedPath); err == nil && info.Mode().IsRegular() && st.ManagedSize == 0 {
		st.ManagedSize = uint64(info.Size())
	}

	if external > 0 {
		st.Note = fmt.Sprintf("将管理普通 Swap 文件 %s；其他 Swap 来源会保留，目标总大小不能小于其他来源的合计大小。", managedPath)
	} else {
		st.Note = fmt.Sprintf("当前由普通 Swap 文件 %s 管理；设置总大小为 0 可关闭并删除该 Swap 文件。", managedPath)
	}
	return st, nil
}

func validateSwapOptions(o SwapApplyOptions) error {
	if o.TotalMB < 0 || o.TotalMB > maxManagedSwapMB {
		return fmt.Errorf("Swap 总大小必须在 0-%d MB", maxManagedSwapMB)
	}
	if o.Swappiness < 0 || o.Swappiness > 100 {
		return errors.New("swappiness 必须在 0-100")
	}
	return nil
}

func setSwappiness(value int) error {
	f, err := os.OpenFile("/proc/sys/vm/swappiness", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("打开 vm.swappiness 失败: %w", err)
	}
	if _, err := f.WriteString(strconv.Itoa(value)); err != nil {
		_ = f.Close()
		return fmt.Errorf("应用 vm.swappiness 失败: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(duiSwapSysctl), 0755); err != nil {
		return err
	}
	content := fmt.Sprintf("# Managed by Dui\nvm.swappiness = %d\n", value)
	if err := os.WriteFile(duiSwapSysctl, []byte(content), 0644); err != nil {
		return fmt.Errorf("持久化 swappiness 失败: %w", err)
	}
	return nil
}

func updateSwapFstab(swapPath string, enable bool) error {
	data, err := os.ReadFile(duiSwapFstab)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	cleanPath := filepath.Clean(swapPath)
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 3 && filepath.Clean(fields[0]) == cleanPath && fields[2] == "swap" {
				continue
			}
		}
		out = append(out, line)
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	if enable {
		out = append(out, cleanPath+" none swap sw 0 0")
	}
	content := strings.Join(out, "\n")
	if content != "" {
		content += "\n"
	}
	return os.WriteFile(duiSwapFstab, []byte(content), 0644)
}

func swapCommand(timeout time.Duration, name string, args ...string) error {
	_, err := runSystemCommand(timeout, name, args...)
	return err
}

func freeBytesForPath(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(filepath.Dir(path), &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func prepareSwapFile(path string, sizeMB int64) error {
	_ = os.Remove(path)
	if sizeMB <= 0 {
		return nil
	}

	if commandExists("fallocate") {
		if err := swapCommand(2*time.Minute, "fallocate", "-l", fmt.Sprintf("%dM", sizeMB), path); err == nil {
			if err := os.Chmod(path, 0600); err != nil {
				return err
			}
			return swapCommand(2*time.Minute, "mkswap", "-f", path)
		}
		_ = os.Remove(path)
	}

	if !commandExists("dd") {
		return errors.New("系统缺少 fallocate/dd，无法创建 Swap 文件")
	}
	if err := swapCommand(10*time.Minute, "dd", "if=/dev/zero", "of="+path, "bs=1M", "count="+strconv.FormatInt(sizeMB, 10), "status=none"); err != nil {
		_ = os.Remove(path)
		return err
	}
	if err := os.Chmod(path, 0600); err != nil {
		_ = os.Remove(path)
		return err
	}
	return swapCommand(2*time.Minute, "mkswap", "-f", path)
}

func swapPathActive(sources []SwapSource, path string) bool {
	clean := filepath.Clean(path)
	for _, src := range sources {
		if src.Type == "file" && filepath.Clean(src.Name) == clean {
			return true
		}
	}
	return false
}

func (s *SwapService) Apply(o SwapApplyOptions) error {
	if err := validateSwapOptions(o); err != nil {
		return err
	}
	if os.Geteuid() != 0 {
		return errors.New("Swap 管理需要 root 权限")
	}
	for _, cmd := range []string{"swapon", "swapoff", "mkswap"} {
		if !commandExists(cmd) {
			return fmt.Errorf("系统缺少 %s", cmd)
		}
	}

	sources, err := readProcSwaps()
	if err != nil {
		return err
	}
	managedPath := chooseManagedSwapPath(sources)
	if managedPath == "" || !filepath.IsAbs(managedPath) || strings.HasPrefix(filepath.Clean(managedPath), "/dev/") {
		return errors.New("无法确定安全的 Swap 文件路径")
	}

	var externalBytes uint64
	var currentManagedBytes uint64
	for _, src := range sources {
		if src.Type == "file" && filepath.Clean(src.Name) == filepath.Clean(managedPath) {
			currentManagedBytes = src.Size
		} else {
			externalBytes += src.Size
		}
	}

	targetBytes := uint64(o.TotalMB) * 1024 * 1024
	if targetBytes < externalBytes {
		return fmt.Errorf("目标 Swap 小于现有其他 Swap（约 %d MB），无法安全缩小", externalBytes/(1024*1024))
	}
	desiredManagedBytes := targetBytes - externalBytes
	desiredManagedMB := int64((desiredManagedBytes + 1024*1024 - 1) / (1024 * 1024))

	if desiredManagedBytes > 0 && currentManagedBytes > 0 {
		diff := int64(currentManagedBytes) - int64(desiredManagedBytes)
		if diff < 0 {
			diff = -diff
		}
		if diff < 1024*1024 {
			if err := updateSwapFstab(managedPath, true); err != nil {
				return err
			}
			return setSwappiness(o.Swappiness)
		}
	}

	active := swapPathActive(sources, managedPath)
	if desiredManagedMB == 0 {
		if active {
			if err := swapCommand(5*time.Minute, "swapoff", managedPath); err != nil {
				return fmt.Errorf("关闭 %s 失败，可能当前 Swap 正在被大量使用: %w", managedPath, err)
			}
		}
		if info, err := os.Lstat(managedPath); err == nil {
			if !info.Mode().IsRegular() {
				if active {
					_ = swapCommand(2*time.Minute, "swapon", managedPath)
				}
				return fmt.Errorf("%s 不是普通文件，拒绝删除", managedPath)
			}
			if err := os.Remove(managedPath); err != nil {
				if active {
					_ = swapCommand(2*time.Minute, "swapon", managedPath)
				}
				return err
			}
		}
		if err := updateSwapFstab(managedPath, false); err != nil {
			return err
		}
		return setSwappiness(o.Swappiness)
	}

	if info, err := os.Lstat(managedPath); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("%s 不是普通文件，拒绝覆盖", managedPath)
	}

	free, err := freeBytesForPath(managedPath)
	if err != nil {
		return fmt.Errorf("读取磁盘剩余空间失败: %w", err)
	}
	if desiredManagedBytes+minFreeReserveBytes > free {
		return fmt.Errorf("磁盘空间不足：安全创建新 Swap 需要约 %d MB，当前可用约 %d MB", desiredManagedMB, free/(1024*1024))
	}

	newPath := managedPath + ".dui-new"
	backupPath := managedPath + ".dui-old"
	_ = os.Remove(newPath)
	_ = os.Remove(backupPath)

	if err := prepareSwapFile(newPath, desiredManagedMB); err != nil {
		return fmt.Errorf("准备新 Swap 文件失败: %w", err)
	}
	defer os.Remove(newPath)

	if active {
		if err := swapCommand(5*time.Minute, "swapoff", managedPath); err != nil {
			return fmt.Errorf("关闭旧 %s 失败，未修改现有 Swap: %w", managedPath, err)
		}
	}

	hadOld := false
	if info, err := os.Lstat(managedPath); err == nil {
		if !info.Mode().IsRegular() {
			if active {
				_ = swapCommand(2*time.Minute, "swapon", managedPath)
			}
			return fmt.Errorf("%s 不是普通文件，拒绝覆盖", managedPath)
		}
		if err := os.Rename(managedPath, backupPath); err != nil {
			if active {
				_ = swapCommand(2*time.Minute, "swapon", managedPath)
			}
			return err
		}
		hadOld = true
	}

	rollback := func() {
		_ = os.Remove(managedPath)
		if hadOld {
			_ = os.Rename(backupPath, managedPath)
			_ = os.Chmod(managedPath, 0600)
			if active {
				_ = swapCommand(2*time.Minute, "swapon", managedPath)
			}
		}
	}

	if err := os.Rename(newPath, managedPath); err != nil {
		rollback()
		return err
	}
	if err := os.Chmod(managedPath, 0600); err != nil {
		rollback()
		return err
	}
	if err := swapCommand(3*time.Minute, "swapon", managedPath); err != nil {
		rollback()
		return fmt.Errorf("启用新 Swap 失败，已尝试恢复旧 Swap: %w", err)
	}
	if err := updateSwapFstab(managedPath, true); err != nil {
		_ = swapCommand(2*time.Minute, "swapoff", managedPath)
		rollback()
		return fmt.Errorf("写入 /etc/fstab 失败，已尝试回滚: %w", err)
	}
	_ = os.Remove(backupPath)
	return setSwappiness(o.Swappiness)
}
