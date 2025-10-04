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
	GetToolPath() string
	GetInstallSource() string
	ExecAndGetInfoString() string
	GetPrintInfoCmd() []string
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

		// Check if this version is installed
		toolFolder := fmt.Sprintf("%s/%s/%s/%s/%s", GetToolFolder(), runtime.GOOS, runtime.GOARCH, toolName, version)
		toolPath := filepath.Join(toolFolder, toolConfig.PathToEntry.Value)

		if _, err := os.Stat(toolPath); err == nil {
			installedVersions = append(installedVersions, version)
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
	toolDir := GetToolFolder()

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
