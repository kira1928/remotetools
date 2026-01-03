package config

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	semver "github.com/blang/semver/v4"
)

type ToolConfig struct {
	ToolName     string
	Version      string
	DownloadURL  OsArchSpecificString `json:"downloadUrl"`
	PathToEntry  OsArchSpecificString `json:"pathToEntry"`
	PrintInfoCmd StringArray          `json:"printInfoCmd,omitempty"`
	// IsExecutable 指示此条目是否为可直接执行的程序（例如 exe/elf）。
	// 对于 any CPU 的 dll、纯资源包等不可直接执行的条目，应设为 false。
	// 若未指定，默认视为 true。
	IsExecutable bool `json:"isExecutable,omitempty"`
}

// OsArchSpecificString 支持字符串或字符串数组（在 JSON 中）以及按 OS/ARCH 嵌套。
// breaking change：仅保留 Values 记录全部候选值，业务代码应通过 Primary() 获取首选值。
type OsArchSpecificString struct {
	Values []string
}

// Primary 返回首选值（通常是列表中的第一个元素）。
func (p OsArchSpecificString) Primary() string {
	if len(p.Values) == 0 {
		return ""
	}
	return p.Values[0]
}

type Config struct {
	ToolConfigs map[string]*ToolConfig `json:"tools"`
}

// StringArray 是对 []string 的轻量封装，支持字符串与数组两种 JSON 形式
type StringArray []string

func (s *StringArray) UnmarshalJSON(data []byte) error {
	// 尝试按数组解析
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*s = arr
		return nil
	}
	// 回退为单一字符串
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if str == "" {
			*s = nil
		} else {
			*s = []string{str}
		}
		return nil
	}
	// 两种都失败，返回原始错误
	return fmt.Errorf("invalid StringArray: %s", string(data))
}

func (p *OsArchSpecificString) UnmarshalJSON(data []byte) (err error) {
	// 解析叶子节点：可能是 string 或 []string
	parseLeaf := func(v interface{}) ([]string, error) {
		switch x := v.(type) {
		case string:
			if x == "" {
				return nil, nil
			}
			return []string{x}, nil
		case []interface{}:
			arr := make([]string, 0, len(x))
			for _, item := range x {
				s, ok := item.(string)
				if !ok || s == "" {
					return nil, fmt.Errorf("array contains non-string or empty element: %v", item)
				}
				arr = append(arr, s)
			}
			return arr, nil
		default:
			return nil, fmt.Errorf("leaf is not string or []string: %v", v)
		}
	}

	// 1. 直接尝试解析为 []string
	var arr []string
	if err = json.Unmarshal(data, &arr); err == nil {
		if len(arr) > 0 {
			p.Values = append(p.Values[:0], arr...)
		}
		return nil
	}

	// 2. 尝试解析为单字符串
	var single string
	if err = json.Unmarshal(data, &single); err == nil {
		if single != "" {
			p.Values = []string{single}
		}
		return nil
	}

	// 3. 尝试解析 OS/ARCH map
	var urlMap map[string]interface{}
	if err = json.Unmarshal(data, &urlMap); err == nil {
		value, ok := urlMap[runtime.GOOS]
		if !ok || value == nil {
			fmt.Printf("no value for %s in %s\n", runtime.GOOS, string(data))
			return nil
		}
		// OS 层可能是叶子或 ARCH map
		if leafVals, lerr := parseLeaf(value); lerr == nil {
			if len(leafVals) > 0 {
				p.Values = leafVals
			}
			return nil
		} else if urlMapForArch, ok2 := value.(map[string]interface{}); ok2 {
			v2, ok3 := urlMapForArch[runtime.GOARCH]
			if !ok3 || v2 == nil {
				fmt.Printf("no value for %s/%s in %s\n", runtime.GOOS, runtime.GOARCH, string(data))
				return nil
			}
			if leafVals, lerr2 := parseLeaf(v2); lerr2 == nil {
				if len(leafVals) > 0 {
					p.Values = leafVals
				}
				return nil
			}
			return fmt.Errorf("value for %s/%s is not valid leaf: %v", runtime.GOOS, runtime.GOARCH, v2)
		}
		return fmt.Errorf("value for %s is not valid leaf or arch map: %v", runtime.GOOS, value)
	}

	// 若都失败，返回最后一次错误（兼容旧行为）
	return nil
}

func LoadConfig(path string) (conf Config, err error) {
	// Read the JSON file
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	return LoadConfigFromBytes(data)
}

func LoadConfigFromBytes(data []byte) (conf Config, err error) {
	// Unmarshal the JSON data into a temporary structure
	// New format: {"toolName": {"version": {"downloadUrl": {}, "pathToEntry": {}}}}
	var tempData map[string]map[string]struct {
		DownloadURL  OsArchSpecificString `json:"downloadUrl"`
		PathToEntry  OsArchSpecificString `json:"pathToEntry"`
		PrintInfoCmd StringArray          `json:"printInfoCmd"`
		// 使用指针以便区分“未提供”与“显式 false”
		IsExecutable *bool `json:"isExecutable,omitempty"`
	}

	err = json.Unmarshal(data, &tempData)
	if err != nil {
		return
	}

	// Convert to the internal structure
	conf.ToolConfigs = make(map[string]*ToolConfig)
	for toolName, versions := range tempData {
		// For each version, create a separate key with toolName@version
		for version, versionData := range versions {
			if versionData.DownloadURL.Primary() == "" {
				fmt.Printf("no download URL for %s/%s in %s@%s\n", runtime.GOOS, runtime.GOARCH, toolName, version)
				continue
			}
			key := toolName + "@" + version
			// 默认 isExecutable = true；若配置显式为 false 则采用 false
			isExec := true
			if versionData.IsExecutable != nil {
				isExec = *versionData.IsExecutable
			}

			conf.ToolConfigs[key] = &ToolConfig{
				ToolName:     toolName,
				Version:      version,
				DownloadURL:  versionData.DownloadURL,
				PathToEntry:  versionData.PathToEntry,
				PrintInfoCmd: versionData.PrintInfoCmd,
				IsExecutable: isExec,
			}
		}
	}

	return
}

// GetLatestVersion returns the latest version string from a list of versions
// It uses semantic version comparison (e.g., "8.0.5" > "8.0.4")
func GetLatestVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	if len(versions) == 1 {
		return versions[0]
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) > 0
	})

	return versions[0]
}

// compareVersions compares two version strings
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if v1 == v2
func compareVersions(v1, v2 string) int {
	// 使用 ParseTolerant 兼容 v 前缀、缺少补零（1 或 1.2）、预发布与构建元数据
	sv1, err1 := semver.ParseTolerant(strings.TrimSpace(v1))
	sv2, err2 := semver.ParseTolerant(strings.TrimSpace(v2))
	if err1 == nil && err2 == nil {
		return sv1.Compare(sv2)
	}
	// 如遇到非标准字符串，做一个保底的字符串比较（尽量避免自实现细节）
	if c := strings.Compare(v1, v2); c < 0 {
		return -1
	} else if c > 0 {
		return 1
	}
	return 0
}

// 保持 strconv 的导入以免 gofmt 误删顺序（其他文件仍使用）。
