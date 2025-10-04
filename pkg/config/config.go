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
	ToolName    string
	Version     string
	DownloadURL OsArchSpecificString `json:"downloadUrl"`
	PathToEntry OsArchSpecificString `json:"pathToEntry"`
}

type OsArchSpecificString struct {
	Value string
}

type Config struct {
	ToolConfigs map[string]*ToolConfig `json:"tools"`
}

func (p *OsArchSpecificString) UnmarshalJSON(data []byte) (err error) {
	// Try to unmarshal the data into a string
	var url string
	err = json.Unmarshal(data, &url)
	if err == nil {
		/*
			"https://xxx"
		*/
		p.Value = url
		return
	}

	// Try to unmarshal the data into a map
	var urlMap map[string]interface{}
	err = json.Unmarshal(data, &urlMap)
	if err == nil {
		value, ok := urlMap[runtime.GOOS]
		if !ok || value == nil {
			return fmt.Errorf("no value for %s in %v", runtime.GOOS, data)
		} else if url, ok := value.(string); ok {
			/*
				{
					"darwin": "https://xxx",
					"linux": "https://xxx",
					"windows": "https://xxx"
				}
			*/
			p.Value = url
			return
		} else if urlMapForArch, ok := value.(map[string]interface{}); ok {
			value, ok := urlMapForArch[runtime.GOARCH]
			if !ok || value == nil {
				return fmt.Errorf("no value for %s/%s in %v", runtime.GOOS, runtime.GOARCH, data)
			} else if url, ok := value.(string); ok {
				/*
					{
						"darwin": ...,
						"linux": ...,
						"windows": {
							"386": "https://xxx",
							"amd64": "https://xxx"
							"arm64": "https://xxx
							"arm": "https://xxx"
						}
					}
				*/
				p.Value = url
				return
			} else {
				return fmt.Errorf("value for %s/%s is not a string: %v", runtime.GOOS, runtime.GOARCH, value)
			}
		} else {
			return fmt.Errorf("value for %s is not a string or a map: %v", runtime.GOOS, value)
		}
	}

	return nil
}

func LoadConfig(path string) (conf Config, err error) {
	// Read the JSON file
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	// Unmarshal the JSON data into a temporary structure
	// New format: {"toolName": {"version": {"downloadUrl": {}, "pathToEntry": {}}}}
	var tempData map[string]map[string]struct {
		DownloadURL OsArchSpecificString `json:"downloadUrl"`
		PathToEntry OsArchSpecificString `json:"pathToEntry"`
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
			key := toolName
			if len(versions) > 1 {
				// If multiple versions exist, use toolName@version as key
				key = toolName + "@" + version
			}
			conf.ToolConfigs[key] = &ToolConfig{
				ToolName:    toolName,
				Version:     version,
				DownloadURL: versionData.DownloadURL,
				PathToEntry: versionData.PathToEntry,
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
