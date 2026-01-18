// Package tools 开发工具覆盖功能
// 允许在开发阶段通过环境变量或配置文件指定本地可执行文件路径，跳过下载

package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// DevToolOverride 定义开发工具覆盖配置
type DevToolOverride struct {
	ToolName string // 工具名称
	ExePath  string // 本地可执行文件路径
}

var (
	devOverrides   = make(map[string]string) // toolName -> exePath
	devOverridesMu sync.RWMutex
)

// SetDevToolOverride 设置开发工具覆盖
// toolName: 工具名称 (如 "klive")
// exePath: 本地可执行文件的绝对路径
func SetDevToolOverride(toolName, exePath string) {
	if toolName == "" || exePath == "" {
		return
	}
	devOverridesMu.Lock()
	devOverrides[toolName] = exePath
	devOverridesMu.Unlock()
}

// GetDevToolOverride 获取开发工具覆盖路径
// 如果没有设置覆盖，返回空字符串
func GetDevToolOverride(toolName string) string {
	devOverridesMu.RLock()
	exePath := devOverrides[toolName]
	devOverridesMu.RUnlock()
	return exePath
}

// ClearDevToolOverride 清除指定工具的开发覆盖
func ClearDevToolOverride(toolName string) {
	devOverridesMu.Lock()
	delete(devOverrides, toolName)
	devOverridesMu.Unlock()
}

// ClearAllDevToolOverrides 清除所有开发覆盖
func ClearAllDevToolOverrides() {
	devOverridesMu.Lock()
	devOverrides = make(map[string]string)
	devOverridesMu.Unlock()
}

// LoadDevToolOverridesFromEnv 从环境变量加载开发工具覆盖
// 环境变量格式: REMOTETOOLS_DEV_<TOOLNAME>=<path>
// 例如: REMOTETOOLS_DEV_KLIVE=/path/to/klive.exe
func LoadDevToolOverridesFromEnv() {
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "REMOTETOOLS_DEV_") {
			continue
		}
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimPrefix(parts[0], "REMOTETOOLS_DEV_")
		toolName := strings.ToLower(key) // 工具名统一小写
		exePath := parts[1]

		if toolName != "" && exePath != "" {
			// 验证路径是否存在
			if _, err := os.Stat(exePath); err == nil {
				SetDevToolOverride(toolName, exePath)
			}
		}
	}
}

// HasDevToolOverride 检查是否有开发覆盖
func HasDevToolOverride(toolName string) bool {
	return GetDevToolOverride(toolName) != ""
}

// DevTool 开发工具包装器，实现 Tool 接口
type DevTool struct {
	toolName string
	exePath  string
}

// NewDevTool 创建开发工具实例
func NewDevTool(toolName, exePath string) *DevTool {
	return &DevTool{
		toolName: toolName,
		exePath:  exePath,
	}
}

func (d *DevTool) DoesToolExist() bool {
	if d.exePath == "" {
		return false
	}
	_, err := os.Stat(d.exePath)
	return err == nil
}

func (d *DevTool) Install() error {
	// 开发工具不需要安装
	return nil
}

func (d *DevTool) Uninstall() error {
	// 开发工具不支持卸载
	return nil
}

func (d *DevTool) Execute(args ...string) error {
	cmd, err := d.CreateExecuteCmd(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (d *DevTool) CreateExecuteCmd(args ...string) (cmd *exec.Cmd, err error) {
	if !d.DoesToolExist() {
		return nil, ErrToolNotFound
	}
	cmd = exec.Command(d.exePath, args...)
	return cmd, nil
}

func (d *DevTool) GetVersion() string {
	return "dev"
}

func (d *DevTool) GetToolName() string {
	return d.toolName
}

func (d *DevTool) GetToolFolder() string {
	return filepath.Dir(d.exePath)
}

func (d *DevTool) GetToolPath() string {
	return d.exePath
}

func (d *DevTool) GetExecFolder() string {
	return filepath.Dir(d.exePath)
}

func (d *DevTool) GetInstallSource() string {
	return "dev-override"
}

func (d *DevTool) ExecAndGetInfoString() string {
	return "开发模式工具: " + d.exePath
}

func (d *DevTool) GetPrintInfoCmd() []string {
	return nil
}

func (d *DevTool) IsFromReadOnlyRootFolder() bool {
	return false
}

func (d *DevTool) GetRootFolder() string {
	return ""
}

// ErrToolNotFound 工具未找到错误
var ErrToolNotFound = &ToolError{Message: "tool not found"}

// ToolError 工具错误
type ToolError struct {
	Message string
}

func (e *ToolError) Error() string {
	return e.Message
}
