package tools

import (
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

// ListTools returns a list of all tools from config
func (a *webuiAdapter) ListTools() ([]webui.ToolInfo, error) {
	var toolsList []webui.ToolInfo

	for _, toolConfig := range a.api.config.ToolConfigs {
		toolInfo := webui.ToolInfo{
			Name:      toolConfig.ToolName,
			Version:   toolConfig.Version,
			Installed: false,
		}

		// Check if tool is installed
		tool, err := a.api.GetToolWithVersion(toolConfig.ToolName, toolConfig.Version)
		if err == nil && tool != nil {
			toolInfo.Installed = tool.DoesToolExist()
		}

		toolsList = append(toolsList, toolInfo)
	}
	// 稳定排序：名称升序；同名按语义化版本升序
	sort.SliceStable(toolsList, func(i, j int) bool {
		if toolsList[i].Name != toolsList[j].Name {
			return toolsList[i].Name < toolsList[j].Name
		}
		vi := strings.TrimSpace(toolsList[i].Version)
		vj := strings.TrimSpace(toolsList[j].Version)
		if svi, err1 := semver.ParseTolerant(vi); err1 == nil {
			if svj, err2 := semver.ParseTolerant(vj); err2 == nil {
				return svi.LT(svj)
			}
		}
		// 解析失败时回退到字符串比较
		return vi < vj
	})

	return toolsList, nil
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
			if progress.Error != nil {
				msg.Error = progress.Error.Error()
			}
			progressCallback(msg)
		})
	}

	// Perform installation
	return tool.Install()
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

// GetToolFolder returns the install folder for a tool version
func (a *webuiAdapter) GetToolFolder(toolName, version string) (string, error) {
	tool, err := a.api.GetToolWithVersion(toolName, version)
	if err != nil {
		return "", err
	}
	return filepath.Dir(tool.GetToolPath()), nil
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
