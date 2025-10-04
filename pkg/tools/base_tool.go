package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"
	"github.com/kira1928/remotetools/pkg/config"
)

type BaseTool struct {
	*config.ToolConfig
}

func NewBaseTool(config *config.ToolConfig) *BaseTool {
	return &BaseTool{
		ToolConfig: config,
	}
}

func (p *BaseTool) GetToolFolder() string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", GetToolFolder(), runtime.GOOS, runtime.GOARCH, p.ToolName, p.Version)
}

func (p *BaseTool) GetToolPath() string {
	return filepath.Join(p.GetToolFolder(), p.PathToEntry.Value)
}

func (p *BaseTool) DoesToolExist() bool {
	_, err := os.Stat(p.GetToolPath())
	return err == nil
}

func (p *BaseTool) Install() error {
	// 在基础层也提供一个占位，具体实现通常由子类覆盖
	// 这里加互斥以防与其他并发操作冲突
	tf := p.GetToolFolder()
	mu := getToolMutex(tf)
	if !mu.TryLock() {
		return ErrToolBusy
	}
	defer mu.Unlock()
	return nil
}

// Uninstall removes the tool by moving it to a trash folder and then deleting it
func (p *BaseTool) Uninstall() error {
	toolFolder := p.GetToolFolder()
	mu := getToolMutex(toolFolder)
	if !mu.TryLock() {
		return ErrToolBusy
	}
	defer mu.Unlock()

	// Check if the tool folder exists
	if _, err := os.Stat(toolFolder); os.IsNotExist(err) {
		return nil // Already uninstalled
	}

	// Get the parent directory of the tool folder
	parentDir := filepath.Dir(toolFolder)

	// Generate a unique trash folder name
	trashFolderName := fmt.Sprintf(".trash-%s-%s", filepath.Base(toolFolder), uuid.New().String())
	trashFolder := filepath.Join(parentDir, trashFolderName)

	// Move the tool folder to the trash folder
	if err := os.Rename(toolFolder, trashFolder); err != nil {
		return fmt.Errorf("failed to move tool folder to trash: %w", err)
	}

	// Try to delete the trash folder
	if err := os.RemoveAll(trashFolder); err != nil {
		// If deletion fails, it's okay - the cleanup function will handle it later
		// Just return success since the tool is no longer accessible
	}

	return nil
}

func (p *BaseTool) CreateExecuteCmd(args ...string) (cmd *exec.Cmd, err error) {
	// check if tool exists
	if !p.DoesToolExist() {
		return nil, fmt.Errorf("tool %s not found", p.ToolName)
	}

	// create the command
	cmd = exec.Command(p.GetToolPath(), args...)

	return
}

func (p *BaseTool) Execute(args ...string) (err error) {
	// create the command
	cmd, err := p.CreateExecuteCmd(args...)
	if err != nil {
		return
	}

	// execute the command
	if err = cmd.Run(); err != nil {
		return err
	}

	return
}

func (p *BaseTool) GetVersion() string {
	return p.Version
}

// ExecAndGetInfoString 运行配置中的 PrintInfoCmd（若存在）并返回其标准输出作为描述信息。
// 若未配置或工具不存在，则返回空字符串。
func (p *BaseTool) ExecAndGetInfoString() string {
	// 若未设置命令，直接返回空
	if len(p.PrintInfoCmd) == 0 {
		return ""
	}
	// 工具必须存在
	if !p.DoesToolExist() {
		return ""
	}
	cmd, err := p.CreateExecuteCmd(p.PrintInfoCmd...)
	if err != nil {
		return ""
	}
	// 捕获标准输出
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func (p *BaseTool) GetPrintInfoCmd() []string {
	return p.PrintInfoCmd
}
