package tools

import (
	"fmt"
	"os/exec"

	"github.com/kira1928/remotetools/pkg/config"
)

type Tool interface {
	DoesToolExist() bool
	Install() error
	Execute(args ...string) error
	CreateExecuteCmd(args ...string) (cmd *exec.Cmd, err error)
	GetVersion() string
	GetToolPath() string
}

var toolFolder string = "external_tools"

func SetToolFolder(folder string) {
	toolFolder = folder
}

func GetToolFolder() string {
	return toolFolder
}

var (
	instance *API
)

type API struct {
	config        config.Config
	toolInstances map[string]Tool
}

func (p *API) LoadConfig(path string) (err error) {
	p.config, err = config.LoadConfig(path)
	return
}

func (p *API) GetTool(toolName string) (tool Tool, err error) {
	var ok bool
	if tool, ok = p.toolInstances[toolName]; ok && tool != nil {
		return
	}

	if p.config.ToolConfigs == nil {
		err = fmt.Errorf("config is not loaded")
		return
	}

	if toolConfig, ok := p.config.ToolConfigs[toolName]; ok {
		tool = NewDownloadTool(toolConfig)
		p.toolInstances[toolName] = tool
	}
	return
}

func init() {
	instance = &API{
		toolInstances: make(map[string]Tool),
	}
}

func Get() *API {
	return instance
}
