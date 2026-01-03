package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	semver "github.com/blang/semver/v4"
	"github.com/kira1928/remotetools/pkg/webui"
)

// webuiAdapter implements webui.APIAdapter to avoid import cycles
type webuiAdapter struct {
	api *API
}

func convertDownloadProcess(dp DownloadProcess) webui.ToolDownloadProcess {
	return webui.ToolDownloadProcess{
		CurrentDownloadURLIndex: dp.CurrentDownloadURLIndex,
		FileSize:                dp.FileSize,
		Status:                  dp.Status,
		AttemptIndex:            dp.AttemptIndex,
		TotalAttempts:           dp.TotalAttempts,
		CurrentURL:              dp.CurrentURL,
		FailedURLs:              copyStringSlice(dp.FailedURLs),
		AllURLs:                 copyStringSlice(dp.AllURLs),
	}
}

// ListToolGroups 返回分组后的工具信息，便于前端按组渲染。
func (a *webuiAdapter) ListToolGroups() ([]webui.ToolGroupOverview, error) {
	configs := a.api.getAllToolConfigs()
	// snapshotMap 先存储组级启用状态，避免多次读取元数据。
	snapshotMap := make(map[string]bool)
	for _, snap := range a.api.ListToolGroups() {
		name := strings.TrimSpace(snap.ToolName)
		if name == "" {
			continue
		}
		snapshotMap[name] = snap.IsEnabled
	}

	groups := make(map[string]*webui.ToolGroupOverview)
	ensureGroup := func(name string) *webui.ToolGroupOverview {
		if g, ok := groups[name]; ok {
			return g
		}
		enabled, ok := snapshotMap[name]
		if !ok {
			enabled = true
		}
		grp := &webui.ToolGroupOverview{
			Name:    name,
			Enabled: enabled,
			Tools:   make([]webui.ToolInfo, 0, 4),
		}
		groups[name] = grp
		return grp
	}

	for _, toolConfig := range configs {
		name := strings.TrimSpace(toolConfig.ToolName)
		if name == "" {
			continue
		}
		toolInfo := webui.ToolInfo{
			Name:         toolConfig.ToolName,
			Version:      toolConfig.Version,
			Installed:    false,
			Preinstalled: false,
			IsExecutable: toolConfig.IsExecutable,
			Enabled:      true,
		}

		// 检查安装状态及运行时元数据。
		tool, err := a.api.GetToolWithVersion(toolConfig.ToolName, toolConfig.Version)
		if err == nil && tool != nil {
			if dt, ok := tool.(*DownloadedTool); ok {
				toolInfo.Enabled = dt.IsEnabled()
				toolInfo.DownloadProcess = convertDownloadProcess(dt.GetDownloadProcess())
				if meta := dt.GetMetadataSnapshot(); meta != nil {
					if data, merr := json.MarshalIndent(meta, "", "  "); merr == nil {
						toolInfo.MetadataJSON = string(data)
					}
				}
			}
			if tool.DoesToolExist() {
				toolInfo.Installed = true
				toolInfo.Preinstalled = tool.IsFromReadOnlyRootFolder()
				toolInfo.StorageFolder = tool.GetToolFolder()
				toolInfo.ExecFolder = tool.GetExecFolder()
				toolInfo.ExecFromTemp = (toolInfo.ExecFolder != "" && toolInfo.StorageFolder != "" && filepath.Clean(toolInfo.ExecFolder) != filepath.Clean(toolInfo.StorageFolder))
			}
		}

		grp := ensureGroup(name)
		grp.Tools = append(grp.Tools, toolInfo)
	}

	// 确保存在纯元数据定义的组也能展示出来（即便当前无版本）。
	for name, enabled := range snapshotMap {
		grp := ensureGroup(name)
		grp.Enabled = enabled
	}

	results := make([]webui.ToolGroupOverview, 0, len(groups))
	for _, grp := range groups {
		sort.SliceStable(grp.Tools, func(i, j int) bool {
			vi := strings.TrimSpace(grp.Tools[i].Version)
			vj := strings.TrimSpace(grp.Tools[j].Version)
			if svi, err1 := semver.ParseTolerant(vi); err1 == nil {
				if svj, err2 := semver.ParseTolerant(vj); err2 == nil {
					return svi.LT(svj)
				}
			}
			return vi < vj
		})
		enabled := grp.Enabled
		if snapEnabled, ok := snapshotMap[grp.Name]; ok {
			enabled = snapEnabled
			grp.Enabled = snapEnabled
		}
		for i := range grp.Tools {
			grp.Tools[i].Enabled = enabled
		}
		results = append(results, *grp)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return results, nil
}

// InstallTool installs a tool with progress reporting
func (a *webuiAdapter) InstallTool(toolName, version string, progressCallback func(webui.ProgressMessage)) error {
	tool, err := a.api.GetToolWithVersion(toolName, version)
	if err != nil {
		return err
	}

	// Set progress callback if it's a DownloadedTool
	if downloadTool, ok := tool.(*DownloadedTool); ok {
		downloadTool.SetProgressCallback(func(progress DownloadProgress) {
			msg := webui.ProgressMessage{
				ToolName:        toolName,
				Version:         version,
				Status:          progress.Status,
				TotalBytes:      progress.TotalBytes,
				DownloadedBytes: progress.DownloadedBytes,
				Speed:           progress.Speed,
			}
			// 透传镜像尝试信息
			msg.AttemptIndex = progress.AttemptIndex
			msg.TotalAttempts = progress.TotalAttempts
			msg.CurrentURL = progress.CurrentURL
			if len(progress.FailedURLs) > 0 {
				msg.FailedURLs = append([]string{}, progress.FailedURLs...)
			}
			if len(progress.AllURLs) > 0 {
				msg.AllURLs = append([]string{}, progress.AllURLs...)
			}
			if progress.Error != nil {
				msg.Error = progress.Error.Error()
			}
			progressCallback(msg)
		})
	}

	// Perform installation
	err = tool.Install()
	if errors.Is(err, ErrDownloadPaused) {
		return nil
	}
	return err
}

// UninstallTool uninstalls a tool
func (a *webuiAdapter) UninstallTool(toolName, version string) error {
	tool, err := a.api.GetToolWithVersion(toolName, version)
	if err != nil {
		return err
	}

	// Perform uninstallation
	return tool.Uninstall()
}

// GetDownloadInfo returns partial download information
func (a *webuiAdapter) GetDownloadInfo(toolName, version string) (int64, int64, error) {
	tool, err := a.api.GetToolWithVersion(toolName, version)
	if err != nil {
		return 0, 0, err
	}
	// Only support DownloadedTool for partial download
	if dt, ok := tool.(*DownloadedTool); ok {
		return dt.GetPartialDownloadInfo()
	}
	return 0, 0, nil
}

// PauseTool triggers pausing download if supported
func (a *webuiAdapter) PauseTool(toolName, version string) error {
	tool, err := a.api.GetToolWithVersion(toolName, version)
	if err != nil {
		return err
	}
	if dt, ok := tool.(*DownloadedTool); ok {
		return dt.Pause()
	}
	return nil
}

// GetToolFolders returns storage and exec folders
func (a *webuiAdapter) GetToolFolders(toolName, version string) (string, string, error) {
	tool, err := a.api.GetToolWithVersion(toolName, version)
	if err != nil {
		return "", "", err
	}
	storage := tool.GetToolFolder()
	execFolder := tool.GetExecFolder()
	if execFolder == "" {
		execFolder = filepath.Dir(tool.GetToolPath())
	}
	return storage, execFolder, nil
}

// GetToolInfoString executes printInfoCmd and returns stdout
func (a *webuiAdapter) GetToolInfoString(toolName, version string) (string, error) {
	tool, err := a.api.GetToolWithVersion(toolName, version)
	if err != nil {
		return "", err
	}
	// 若未安装或未配置命令，返回空字符串
	return tool.ExecAndGetInfoString(), nil
}

// ListActiveInstalls returns the active installs in the form of tool@version
func (a *webuiAdapter) ListActiveInstalls() []string {
	return listActiveDownloads()
}

// SetToolGroupEnabled 切换指定工具组的启用状态。
func (a *webuiAdapter) SetToolGroupEnabled(toolName string, enabled bool) error {
	return a.api.SetToolGroupEnabled(toolName, enabled)
}

// SetToolEnabled 兼容旧接口，内部转发到工具组级别的开关。
func (a *webuiAdapter) SetToolEnabled(toolName, _ string, enabled bool) error {
	return a.SetToolGroupEnabled(toolName, enabled)
}

// GetToolMetadata 返回格式化的元数据 JSON 字符串
func (a *webuiAdapter) GetToolMetadata(toolName, version string) (string, error) {
	tool, err := a.api.GetToolWithVersion(toolName, version)
	if err != nil {
		return "", err
	}
	dt, ok := tool.(*DownloadedTool)
	if !ok {
		return "", fmt.Errorf("tool %s@%s does not expose metadata", toolName, version)
	}
	meta := dt.GetMetadataSnapshot()
	if meta == nil {
		return "", nil
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
