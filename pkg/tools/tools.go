package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kira1928/remotetools/pkg/config"
	"github.com/kira1928/remotetools/pkg/webui"
)

type Tool interface {
	DoesToolExist() bool
	Install() error
	Uninstall() error
	Execute(args ...string) error
	CreateExecuteCmd(args ...string) (cmd *exec.Cmd, err error)
	GetVersion() string
	// GetToolName 返回工具名称（如 "dotnet"）
	GetToolName() string
	// GetToolFolder 返回该工具实际所在的目录（若未发现，返回空字符串）
	GetToolFolder() string
	// GetToolPath 返回实际可执行入口路径；若不存在则返回空字符串
	GetToolPath() string
	// GetExecFolder 返回执行使用的目录；当可写目录不可执行时，可能为临时目录
	GetExecFolder() string
	GetInstallSource() string
	ExecAndGetInfoString() string
	GetPrintInfoCmd() []string
	// IsFromReadOnlyRootFolder 返回该工具是否是从只读目录（ReadOnlyRootFolders）中识别到
	IsFromReadOnlyRootFolder() bool
	// GetRootFolder 返回该工具所属的根目录（只读根或可写根）。若未发现，返回空字符串
	GetRootFolder() string
}

// 可写入的根目录（用于下载/解压/卸载）
var rootFolder string = "external_tools"

// 只读根目录列表（优先查找）。例如容器内预置目录。
var roRootFolders []string

// 可执行权限的临时目录：当 rootFolder 所在挂载点不支持 exec 时，工具会复制到此处运行
var tmpExecRootFolder string

// SetRootFolder 设置可写入的根目录
func SetRootFolder(folder string) {
	rootFolder = folder
}

// GetRootFolder 返回可写入的根目录
func GetRootFolder() string {
	return rootFolder
}

// SetTmpRootFolderForExecPermission 设置在可写目录不可执行时用于运行的临时目录
func SetTmpRootFolderForExecPermission(folder string) {
	tmpExecRootFolder = folder
}

// GetTmpRootFolderForExecPermission 返回当前配置的临时执行目录
func GetTmpRootFolderForExecPermission() string {
	return tmpExecRootFolder
}

// isExecSupported 在 Linux 上尝试在目标目录直接创建并执行脚本，判断是否被 noexec/权限限制。
// 其他平台上默认返回 true。
type execSupportEntry struct {
	ok        bool
	checkedAt time.Time
}

var execSupportCache sync.Map // key: cleaned dir path, value: execSupportEntry
const execSupportTTL = 10 * time.Minute

// isExecSupported returns whether execution is supported in the directory.
// On Linux, it writes and executes a tiny script; on other platforms, returns true.
func isExecSupported(dir string) bool {
	if dir == "" {
		return false
	}
	if runtime.GOOS != "linux" {
		return true
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	testFile := filepath.Join(dir, ".rt_exec_test.sh")
	content := []byte("#!/bin/sh\necho ok\n")
	if err := os.WriteFile(testFile, content, 0o755); err != nil {
		return false
	}
	defer os.Remove(testFile)
	cmd := exec.Command(testFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "ok")
}

// isExecSupportedCached caches the result to avoid repeated expensive checks for the same folder.
func isExecSupportedCached(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	key := filepath.Clean(dir)
	if v, ok := execSupportCache.Load(key); ok {
		if ent, ok2 := v.(execSupportEntry); ok2 {
			if time.Since(ent.checkedAt) < execSupportTTL {
				return ent.ok
			}
		}
	}
	ok := isExecSupported(key)
	execSupportCache.Store(key, execSupportEntry{ok: ok, checkedAt: time.Now()})
	return ok
}

// copyDir 递归复制目录内容（覆盖已存在文件）
func copyDir(src, dst string) error {
	// ensure dst exists
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		info, err := os.Lstat(s)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// skip symlinks for safety
			continue
		}
		if info.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		if err := os.WriteFile(d, data, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

// SetReadOnlyRootFolders 设置只读根目录列表（查找顺序按给定顺序）。
func SetReadOnlyRootFolders(folders []string) {
	roRootFolders = make([]string, 0, len(folders))
	for _, f := range folders {
		if f == "" {
			continue
		}
		roRootFolders = append(roRootFolders, f)
	}
}

// AddReadOnlyRootFolder 追加一个只读根目录（在查找链末尾）。
func AddReadOnlyRootFolder(folder string) {
	if folder == "" {
		return
	}
	roRootFolders = append(roRootFolders, folder)
}

// GetReadOnlyRootFolders 返回当前配置的只读根目录列表。
func GetReadOnlyRootFolders() []string {
	return append([]string(nil), roRootFolders...)
}

// getCandidateRootFolders 返回按优先级排序的工具根目录：先只读目录们，最后是可写目录
func getCandidateRootFolders() []string {
	roots := make([]string, 0, len(roRootFolders)+1)
	roots = append(roots, roRootFolders...)
	roots = append(roots, GetRootFolder())
	return roots
}

var (
	instance *API
)

type API struct {
	config        config.Config
	toolInstances map[string]Tool
	webUIServer   *webui.WebUIServer
	// 保护 toolInstances 的并发读写
	toolMu sync.RWMutex
}

func (p *API) LoadConfig(path string) (err error) {
	p.config, err = config.LoadConfig(path)
	return
}

func (p *API) LoadConfigFromBytes(data []byte) (err error) {
	p.config, err = config.LoadConfigFromBytes(data)
	return
}

func (p *API) GetTool(toolName string) (tool Tool, err error) {
	return p.GetToolAuto(toolName, AutoVersionPreferInstalled)
}

// AutoVersionStrategy defines the strategy for automatic version selection
type AutoVersionStrategy int

const (
	// AutoVersionPreferInstalled prefers the highest installed version, falls back to latest available
	AutoVersionPreferInstalled AutoVersionStrategy = iota
	// AutoVersionLatestAvailable always uses the latest version from config
	AutoVersionLatestAvailable
	// AutoVersionOnlyInstalled only uses installed versions, returns error if none installed
	AutoVersionOnlyInstalled
)

// GetToolAuto gets a tool with automatic version selection based on the strategy
func (p *API) GetToolAuto(toolName string, strategy AutoVersionStrategy) (tool Tool, err error) {
	// First check if there's a direct match (single version case)
	ok := false
	p.toolMu.RLock()
	tool, ok = p.toolInstances[toolName]
	p.toolMu.RUnlock()
	if ok && tool != nil {
		return tool, nil
	}

	if p.config.ToolConfigs == nil {
		return nil, fmt.Errorf("config is not loaded")
	}

	// Check for direct key match (single version)
	if toolConfig, ok := p.config.ToolConfigs[toolName]; ok {
		// 双重检查，避免重复创建
		p.toolMu.RLock()
		if cachedTool, exists := p.toolInstances[toolName]; exists && cachedTool != nil {
			p.toolMu.RUnlock()
			return cachedTool, nil
		}
		p.toolMu.RUnlock()

		tool = NewDownloadTool(toolConfig)
		p.toolMu.Lock()
		// 二次判断，防止并发创建
		if cachedTool, exists := p.toolInstances[toolName]; exists && cachedTool != nil {
			p.toolMu.Unlock()
			return cachedTool, nil
		}
		p.toolInstances[toolName] = tool
		p.toolMu.Unlock()
		return tool, nil
	}

	// Find all versions of this tool
	var availableVersions []string
	for key := range p.config.ToolConfigs {
		if strings.HasPrefix(key, toolName+"@") {
			version := strings.TrimPrefix(key, toolName+"@")
			availableVersions = append(availableVersions, version)
		}
	}

	if len(availableVersions) == 0 {
		return nil, fmt.Errorf("tool %s not found in config", toolName)
	}

	var selectedVersion string

	switch strategy {
	case AutoVersionPreferInstalled:
		// Try to find the highest installed version
		installedVersion := p.getHighestInstalledVersion(toolName, availableVersions)
		if installedVersion != "" {
			selectedVersion = installedVersion
		} else {
			// Fall back to latest available version
			selectedVersion = config.GetLatestVersion(availableVersions)
		}

	case AutoVersionLatestAvailable:
		// Always use the latest version from config
		selectedVersion = config.GetLatestVersion(availableVersions)

	case AutoVersionOnlyInstalled:
		// Only use installed versions
		installedVersion := p.getHighestInstalledVersion(toolName, availableVersions)
		if installedVersion == "" {
			return nil, fmt.Errorf("no installed version of tool %s found", toolName)
		}
		selectedVersion = installedVersion
	}

	return p.GetToolWithVersion(toolName, selectedVersion)
}

// getHighestInstalledVersion finds the highest version that is already installed locally
func (p *API) getHighestInstalledVersion(toolName string, versions []string) string {
	var installedVersions []string

	for _, version := range versions {
		key := toolName + "@" + version
		toolConfig, ok := p.config.ToolConfigs[key]
		if !ok {
			continue
		}
		// 在所有候选根目录中查找是否存在该版本
		for _, root := range getCandidateRootFolders() {
			toolFolder := generateToolFolderPath(root, toolName, version)
			toolPath := filepath.Join(toolFolder, toolConfig.PathToEntry.Value)
			if _, err := os.Stat(toolPath); err == nil {
				installedVersions = append(installedVersions, version)
				break
			}
		}
	}

	if len(installedVersions) == 0 {
		return ""
	}

	return config.GetLatestVersion(installedVersions)
}

// GetToolLatest gets the latest version of a tool from config (for downloading)
func (p *API) GetToolLatest(toolName string) (tool Tool, err error) {
	return p.GetToolAuto(toolName, AutoVersionLatestAvailable)
}

// GetToolInstalled gets the highest installed version of a tool (for execution)
func (p *API) GetToolInstalled(toolName string) (tool Tool, err error) {
	return p.GetToolAuto(toolName, AutoVersionOnlyInstalled)
}

func (p *API) GetToolWithVersion(toolName, version string) (tool Tool, err error) {
	key := toolName + "@" + version

	var ok bool
	p.toolMu.RLock()
	tool, ok = p.toolInstances[key]
	p.toolMu.RUnlock()
	if ok && tool != nil {
		return
	}

	if p.config.ToolConfigs == nil {
		err = fmt.Errorf("config is not loaded")
		return
	}

	if toolConfig, ok := p.config.ToolConfigs[key]; ok {
		// 双重检查：创建前后都判断缓存
		p.toolMu.RLock()
		if cached, exists := p.toolInstances[key]; exists && cached != nil {
			p.toolMu.RUnlock()
			tool = cached
			return
		}
		p.toolMu.RUnlock()

		t := NewDownloadTool(toolConfig)
		p.toolMu.Lock()
		if cached, exists := p.toolInstances[key]; exists && cached != nil {
			tool = cached
			p.toolMu.Unlock()
			return
		}
		p.toolInstances[key] = t
		p.toolMu.Unlock()
		tool = t
	}
	return
}

// CleanupTrash removes any leftover .trash-* folders in the tool directory
func CleanupTrash() {
	toolDir := GetRootFolder()

	// Check if the tool directory exists
	if _, err := os.Stat(toolDir); os.IsNotExist(err) {
		return
	}

	// Walk through the tool directory
	filepath.Walk(toolDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors and continue
		}

		// Check if it's a directory and starts with .trash-
		if info.IsDir() && strings.HasPrefix(info.Name(), ".trash-") {
			// Try to remove it, but don't fail if we can't
			os.RemoveAll(path)
		}

		return nil
	})
}

func init() {
	instance = &API{
		toolInstances: make(map[string]Tool),
		webUIServer:   webui.NewWebUIServer(),
	}
	// Set the webui adapter to avoid import cycles
	webui.SetAPIAdapter(&webuiAdapter{api: instance})

	// Cleanup any leftover trash folders on startup
	CleanupTrash()
}

func Get() *API {
	return instance
}

// GetConfig returns the current configuration
func (p *API) GetConfig() config.Config {
	return p.config
}

// StartWebUI starts the web UI server
// If port is 0, a random available port will be chosen
func (p *API) StartWebUI(port int) error {
	return p.webUIServer.Start(port)
}

// StopWebUI stops the web UI server
func (p *API) StopWebUI() error {
	return p.webUIServer.Stop()
}

// GetWebUIStatus returns the current status of the web UI server
func (p *API) GetWebUIStatus() webui.ServerStatus {
	return p.webUIServer.GetStatus()
}

// GetWebUIPort returns the port the web UI server is running on
// Returns 0 if the server is not running
func (p *API) GetWebUIPort() int {
	return p.webUIServer.GetPort()
}

func (p *API) GetWebUIAddresses() (addresses []string, err error) {
	return p.webUIServer.GetAddresses()
}

// DeleteUnknownToolsInRoot 清理可写根目录下的工具：
// - 对于非当前 OS 或 ARCH 的目录，直接整目录删除（不深入遍历）。
// - 对于当前 OS/ARCH，删除所有不在当前配置(p.config)中的 工具@版本 目录。
// 仅作用于可写根目录（GetRootFolder），不会触及只读根目录。
// 返回被删除的目录完整路径列表（可能是版本目录、架构目录或系统目录）。
func (p *API) DeleteUnknownToolsInRoot() (deleted []string, err error) {
	// 配置必须已加载
	if p.config.ToolConfigs == nil {
		return nil, fmt.Errorf("config is not loaded")
	}

	// 构建允许集合（key 形如 tool@version）
	allowed := make(map[string]struct{}, len(p.config.ToolConfigs))
	for key := range p.config.ToolConfigs {
		allowed[key] = struct{}{}
	}

	root := GetRootFolder()

	// 遍历所有 OS/ARCH 目录层级：root/os/arch/tool/version
	osDirs, err := os.ReadDir(root)
	if err != nil {
		// 根不存在或不可读则视为无事可做
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var firstErr error

	for _, osd := range osDirs {
		if !osd.IsDir() {
			continue
		}
		osPath := filepath.Join(root, osd.Name())
		// 非当前 OS：整目录删除
		if osd.Name() != runtime.GOOS {
			if remErr := os.RemoveAll(osPath); remErr != nil {
				if firstErr == nil {
					firstErr = remErr
				}
			} else {
				deleted = append(deleted, osPath)
			}
			continue
		}

		archDirs, _ := os.ReadDir(osPath)
		for _, ad := range archDirs {
			if !ad.IsDir() {
				continue
			}
			archPath := filepath.Join(osPath, ad.Name())
			// 非当前 ARCH：整目录删除
			if ad.Name() != runtime.GOARCH {
				if remErr := os.RemoveAll(archPath); remErr != nil {
					if firstErr == nil {
						firstErr = remErr
					}
				} else {
					deleted = append(deleted, archPath)
				}
				continue
			}

			// 遍历工具 -> 版本
			toolDirs, _ := os.ReadDir(archPath)
			for _, td := range toolDirs {
				if !td.IsDir() {
					continue
				}
				toolName := td.Name()
				toolPath := filepath.Join(archPath, toolName)

				versionDirs, _ := os.ReadDir(toolPath)
				for _, vd := range versionDirs {
					if !vd.IsDir() {
						continue
					}
					version := vd.Name()
					// 跳过临时/垃圾目录
					if strings.HasPrefix(version, ".tmp_") || strings.HasPrefix(version, ".trash-") {
						continue
					}

					key := toolName + "@" + version
					if _, ok := allowed[key]; ok {
						// 在配置中，保留
						continue
					}

					// 不在配置中：尝试加锁并删除
					versionPath := filepath.Join(toolPath, version)
					mu := getToolMutex(versionPath)
					if !mu.TryLock() {
						// 忙碌则跳过，不视为致命错误
						continue
					}
					func() {
						defer mu.Unlock()
						if remErr := os.RemoveAll(versionPath); remErr != nil {
							if firstErr == nil {
								firstErr = remErr
							}
						} else {
							deleted = append(deleted, versionPath)
						}
					}()
				}

				// 版本目录处理后，如工具目录已空则尝试清理该工具目录
				if entries, _ := os.ReadDir(toolPath); len(entries) == 0 {
					_ = os.Remove(toolPath)
				}
			}

			// 工具目录处理后，如架构目录已空则尝试清理该架构目录
			if entries, _ := os.ReadDir(archPath); len(entries) == 0 {
				_ = os.Remove(archPath)
			}
		}

		// 架构目录处理后，如操作系统目录已空则尝试清理该目录
		if entries, _ := os.ReadDir(osPath); len(entries) == 0 {
			_ = os.Remove(osPath)
		}
	}

	return deleted, firstErr
}

// DeleteAllExceptToolsInRoot 删除可写根目录中：
// - 对于非当前 OS 或 ARCH 的目录，直接整目录删除（不深入遍历）。
// - 对于当前 OS/ARCH，仅保留 toKeep 中列出的 工具@版本，其余删除。
// 仅作用于可写根目录（GetRootFolder），不会触及只读根目录。
// 返回被删除的目录完整路径列表（可能是版本目录、架构目录或系统目录）。
func (p *API) DeleteAllExceptToolsInRoot(toKeep []Tool) (deleted []string, err error) {
	// 允许集合：来自调用方指定的工具列表（key 形如 tool@version）
	allowed := make(map[string]struct{}, len(toKeep))
	for _, t := range toKeep {
		if t == nil {
			continue
		}
		name := strings.TrimSpace(t.GetToolName())
		ver := strings.TrimSpace(t.GetVersion())
		if name == "" || ver == "" {
			continue
		}
		allowed[name+"@"+ver] = struct{}{}
	}

	root := GetRootFolder()

	osDirs, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var firstErr error

	for _, osd := range osDirs {
		if !osd.IsDir() {
			continue
		}
		osPath := filepath.Join(root, osd.Name())
		// 非当前 OS：整目录删除
		if osd.Name() != runtime.GOOS {
			if remErr := os.RemoveAll(osPath); remErr != nil {
				if firstErr == nil {
					firstErr = remErr
				}
			} else {
				deleted = append(deleted, osPath)
			}
			continue
		}

		archDirs, _ := os.ReadDir(osPath)
		for _, ad := range archDirs {
			if !ad.IsDir() {
				continue
			}
			archPath := filepath.Join(osPath, ad.Name())
			// 非当前 ARCH：整目录删除
			if ad.Name() != runtime.GOARCH {
				if remErr := os.RemoveAll(archPath); remErr != nil {
					if firstErr == nil {
						firstErr = remErr
					}
				} else {
					deleted = append(deleted, archPath)
				}
				continue
			}

			toolDirs, _ := os.ReadDir(archPath)
			for _, td := range toolDirs {
				if !td.IsDir() {
					continue
				}
				toolName := td.Name()
				toolPath := filepath.Join(archPath, toolName)

				versionDirs, _ := os.ReadDir(toolPath)
				for _, vd := range versionDirs {
					if !vd.IsDir() {
						continue
					}
					version := vd.Name()
					if strings.HasPrefix(version, ".tmp_") || strings.HasPrefix(version, ".trash-") {
						continue
					}
					key := toolName + "@" + version
					if _, ok := allowed[key]; ok {
						// 保留
						continue
					}

					versionPath := filepath.Join(toolPath, version)
					mu := getToolMutex(versionPath)
					if !mu.TryLock() {
						// 忙碌则跳过
						continue
					}
					func() {
						defer mu.Unlock()
						if remErr := os.RemoveAll(versionPath); remErr != nil {
							if firstErr == nil {
								firstErr = remErr
							}
						} else {
							deleted = append(deleted, versionPath)
						}
					}()
				}

				// 若已空则移除工具目录
				if entries, _ := os.ReadDir(toolPath); len(entries) == 0 {
					_ = os.Remove(toolPath)
				}
			}

			// 若已空则移除 arch 目录
			if entries, _ := os.ReadDir(archPath); len(entries) == 0 {
				_ = os.Remove(archPath)
			}
		}

		// 若已空则移除 os 目录
		if entries, _ := os.ReadDir(osPath); len(entries) == 0 {
			_ = os.Remove(osPath)
		}
	}

	return deleted, firstErr
}
