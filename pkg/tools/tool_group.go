package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// ToolGroupMetadata 表示工具组级别的元数据，只记录必要的启用状态。
type ToolGroupMetadata struct {
	ToolName  string `json:"toolName"`
	IsEnabled bool   `json:"isEnabled"`
}

// ToolGroupSnapshot 用于向外部暴露工具组的关键信息。
type ToolGroupSnapshot struct {
	ToolName  string `json:"toolName"`
	IsEnabled bool   `json:"isEnabled"`
}

// ToolGroup 管理同名不同版本工具的共享状态（例如启用/禁用）。
type ToolGroup struct {
	name       string
	metadata   *ToolGroupMetadata
	metadataMu sync.Mutex
}

// newToolGroup 构建一个新的工具组实例，实际元数据会在首次访问时加载或创建。
func newToolGroup(name string) *ToolGroup {
	return &ToolGroup{name: name}
}

// metadataPath 返回当前工具组元数据文件的绝对路径。
func (g *ToolGroup) metadataPath() string {
	root := GetRootFolder()
	if strings.TrimSpace(root) == "" {
		root = "external_tools"
	}
	file := fmt.Sprintf("%s.json", g.name)
	return filepath.Join(root, runtime.GOOS, runtime.GOARCH, "_groups", file)
}

// ensureMetadataLocked 在持有互斥锁的情况下加载或初始化元数据。
func (g *ToolGroup) ensureMetadataLocked() (*ToolGroupMetadata, error) {
	if g.metadata != nil {
		return g.metadata, nil
	}
	path := g.metadataPath()
	data, err := os.ReadFile(path)
	if err == nil {
		var meta ToolGroupMetadata
		if jsonErr := json.Unmarshal(data, &meta); jsonErr == nil {
			if strings.TrimSpace(meta.ToolName) == "" {
				meta.ToolName = g.name
			}
			g.metadata = &meta
			return g.metadata, nil
		}
	}
	meta := &ToolGroupMetadata{ToolName: g.name, IsEnabled: true}
	g.metadata = meta
	if writeErr := g.persistLocked(); writeErr != nil {
		return meta, writeErr
	}
	return meta, nil
}

// persistLocked 写入最新的元数据到磁盘，仅在持有互斥锁时调用。
func (g *ToolGroup) persistLocked() error {
	if g.metadata == nil {
		return nil
	}
	path := g.metadataPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(g.metadata, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// IsEnabled 返回工具组当前是否启用；读取失败时默认返回 true。
func (g *ToolGroup) IsEnabled() bool {
	g.metadataMu.Lock()
	defer g.metadataMu.Unlock()
	meta, err := g.ensureMetadataLocked()
	if err != nil || meta == nil {
		return true
	}
	return meta.IsEnabled
}

// SetEnabled 更新工具组的启用状态，仅当状态发生改变时才写入磁盘。
func (g *ToolGroup) SetEnabled(enabled bool) error {
	g.metadataMu.Lock()
	defer g.metadataMu.Unlock()
	meta, err := g.ensureMetadataLocked()
	if err != nil {
		return err
	}
	if meta.IsEnabled == enabled {
		return nil
	}
	meta.IsEnabled = enabled
	return g.persistLocked()
}

// Snapshot 返回当前工具组的只读快照，用于对外展示。
func (g *ToolGroup) Snapshot() ToolGroupSnapshot {
	g.metadataMu.Lock()
	defer g.metadataMu.Unlock()
	if meta, err := g.ensureMetadataLocked(); err == nil && meta != nil {
		return ToolGroupSnapshot{ToolName: meta.ToolName, IsEnabled: meta.IsEnabled}
	}
	return ToolGroupSnapshot{ToolName: g.name, IsEnabled: true}
}

// Name 返回工具组名称。
func (g *ToolGroup) Name() string {
	return g.name
}
