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

// GetToolFolder 返回该工具实际所在的目录；若未找到则返回空字符串
func (p *BaseTool) GetToolFolder() string {
	if _, folder, ok := p.resolveExistingPath(); ok {
		return folder
	}
	return ""
}

// GetWritableToolFolder 返回用于安装/卸载的可写目录（不做存在性判断）
func (p *BaseTool) GetWritableToolFolder() string {
	return p.GetToolFolderPath(GetRootFolder())
}

func generateToolFolderPath(rootFolder, toolName, version string) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", rootFolder, runtime.GOOS, runtime.GOARCH, toolName, version)
}

func (p *BaseTool) GetToolFolderPath(rootFolder string) string {
	return generateToolFolderPath(rootFolder, p.ToolName, p.Version)
}

// resolveExistingPath 在候选根目录（先只读，后可写）中查找已存在的可执行路径
// 返回：entryPath, folderPath, found
func (p *BaseTool) resolveExistingPath() (string, string, bool) {
	for _, root := range getCandidateRootFolders() {
		folder := p.GetToolFolderPath(root)
		entry := filepath.Join(folder, p.PathToEntry.Value)
		if _, err := os.Stat(entry); err == nil {
			return entry, folder, true
		}
	}
	return "", "", false
}

// GetToolPath 若存在于任意候选目录，则返回实际存在的入口路径；否则返回可写目录中的预期路径
func (p *BaseTool) GetToolPath() string {
	// 优先返回可执行的路径：当 rootFolder 不可执行时复制到 tmpExecFolder 运行
	entry, folder, ok := p.resolveExistingPath()
	if !ok {
		return ""
	}
	// 若未设置 tmp 目录或当前目录可执行，直接返回原路径
	if tmpExecRootFolder == "" || isExecSupportedCached(folder) {
		return entry
	}
	// 需要复制到临时目录
	execFolder := p.GetExecFolder()
	if execFolder == "" {
		return entry // 回退：若无法确定，仍返回原路径
	}
	// 确保已复制（若不存在则复制）
	if st, err := os.Stat(execFolder); err != nil || !st.IsDir() {
		_ = os.MkdirAll(execFolder, 0o755)
		_ = copyDir(folder, execFolder)
	}
	return filepath.Join(execFolder, p.PathToEntry.Value)
}

// DoesToolExist 在候选根目录中检查是否已存在
func (p *BaseTool) DoesToolExist() bool {
	_, _, ok := p.resolveExistingPath()
	return ok
}

// GetResolvedEntryPath 若在候选目录中找到，返回实际存在的入口路径；否则返回空字符串
// GetRootFolder 返回该工具被发现时所在的根目录（只读根或可写根）。若未发现返回空字符串
func (p *BaseTool) GetRootFolder() string {
	_, folder, ok := p.resolveExistingPath()
	if !ok || folder == "" {
		return ""
	}
	// 依次检查每个根是否与该 folder 匹配
	for _, root := range getCandidateRootFolders() {
		// 根据 root 构造期望路径
		expected := p.GetToolFolderPath(root)
		if filepath.Clean(expected) == filepath.Clean(folder) {
			return root
		}
	}
	return ""
}

// IsFromReadOnlyRootFolder 返回是否来自只读根目录
func (p *BaseTool) IsFromReadOnlyRootFolder() bool {
	root := p.GetRootFolder()
	if root == "" {
		return false
	}
	// 在只读根列表中匹配
	for _, ro := range GetReadOnlyRootFolders() {
		if filepath.Clean(ro) == filepath.Clean(root) {
			return true
		}
	}
	return false
}

func (p *BaseTool) Install() error {
	// 在基础层也提供一个占位，具体实现通常由子类覆盖
	// 这里加互斥以防与其他并发操作冲突
	tf := p.GetWritableToolFolder()
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

	// 同步清理临时执行目录（若存在）
	if tmp := p.GetToolFolderPath(GetTmpRootFolderForExecPermission()); tmp != "" {
		_ = os.RemoveAll(tmp)
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

// GetToolName 返回工具名称
func (p *BaseTool) GetToolName() string {
	return p.ToolName
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
	// 捕获标准输出和错误输出
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

func (p *BaseTool) GetPrintInfoCmd() []string {
	return p.PrintInfoCmd
}

// GetExecFolder 返回执行所用目录：当可写目录不可执行且配置了临时执行目录时，返回临时目录中的副本路径
func (p *BaseTool) GetExecFolder() string {
	// 先确认存储位置
	_, folder, ok := p.resolveExistingPath()
	if !ok || folder == "" {
		return ""
	}
	if tmpExecRootFolder == "" || isExecSupportedCached(folder) {
		return folder
	}
	// 计算临时执行目录
	if tmpExecRootFolder == "" {
		return folder
	}
	return p.GetToolFolderPath(tmpExecRootFolder)
}
