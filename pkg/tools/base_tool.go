package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

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
	return fmt.Sprintf("%s/%s/%s/%s", GetToolFolder(), runtime.GOOS, runtime.GOARCH, p.ToolName)
}

func (p *BaseTool) GetToolPath() string {
	return filepath.Join(p.GetToolFolder(), p.PathToEntry.Value)
}

func (p *BaseTool) DoesToolExist() bool {
	_, err := os.Stat(p.GetToolPath())
	return err == nil
}

func (p *BaseTool) Install() error {
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
