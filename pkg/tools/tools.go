package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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

// SetRootFolder 设置可写入的根目录
func SetRootFolder(folder string) {
	rootFolder = folder
}

// GetRootFolder 返回可写入的根目录
func GetRootFolder() string {
	return rootFolder
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
	if tool, ok = p.toolInstances[toolName]; ok && tool != nil {
		return tool, nil
	}

	if p.config.ToolConfigs == nil {
		return nil, fmt.Errorf("config is not loaded")
	}

	// Check for direct key match (single version)
	if toolConfig, ok := p.config.ToolConfigs[toolName]; ok {
		if cachedTool, ok := p.toolInstances[toolName]; ok && cachedTool != nil {
			return cachedTool, nil
		}
		tool = NewDownloadTool(toolConfig)
		p.toolInstances[toolName] = tool
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
	if tool, ok = p.toolInstances[key]; ok && tool != nil {
		return
	}

	if p.config.ToolConfigs == nil {
		err = fmt.Errorf("config is not loaded")
		return
	}

	if toolConfig, ok := p.config.ToolConfigs[key]; ok {
		tool = NewDownloadTool(toolConfig)
		p.toolInstances[key] = tool
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

// DeleteUnknownToolsInRoot 扫描可写根目录下当前 OS/ARCH 的工具目录，
// 删除所有不在当前配置(p.config)中的 工具@版本 目录。
// 仅作用于可写根目录（GetRootFolder），不会触及只读根目录。
// 返回被删除的版本目录的完整路径列表。
func (p *API) DeleteUnknownToolsInRoot() (deleted []string, err error) {
	// 配置必须已加载
	if p.config.ToolConfigs == nil {
		return nil, fmt.Errorf("config is not loaded")
	}

	// 构建允许集合（仅针对当前 OS/ARCH 已有可用下载地址的配置）
	allowed := make(map[string]struct{}, len(p.config.ToolConfigs))
	for key := range p.config.ToolConfigs {
		// 形如 tool@version
		allowed[key] = struct{}{}
	}

	root := GetRootFolder()
	base := filepath.Join(root, runtime.GOOS, runtime.GOARCH)

	// 根不存在则无事可做
	if stat, statErr := os.Stat(base); statErr != nil || !stat.IsDir() {
		return nil, nil
	}

	// 遍历工具 -> 版本
	toolDirs, readErr := os.ReadDir(base)
	if readErr != nil {
		return nil, readErr
	}

	var firstErr error
	for _, td := range toolDirs {
		if !td.IsDir() {
			continue
		}
		toolName := td.Name()
		toolPath := filepath.Join(base, toolName)

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

	return deleted, firstErr
}

// DeleteAllExceptToolsInRoot 删除可写根目录中，除 toKeep 中列出的 工具@版本 以外的所有其他目录。
// 仅作用于可写根目录（GetRootFolder），不会触及只读根目录。
// 返回被删除的版本目录的完整路径列表。
func (p *API) DeleteAllExceptToolsInRoot(toKeep []Tool) (deleted []string, err error) {
	// 允许集合：来自调用方指定的工具列表
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
	base := filepath.Join(root, runtime.GOOS, runtime.GOARCH)

	if stat, statErr := os.Stat(base); statErr != nil || !stat.IsDir() {
		return nil, nil
	}

	toolDirs, readErr := os.ReadDir(base)
	if readErr != nil {
		return nil, readErr
	}

	var firstErr error
	for _, td := range toolDirs {
		if !td.IsDir() {
			continue
		}
		toolName := td.Name()
		toolPath := filepath.Join(base, toolName)

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

	return deleted, firstErr
}
