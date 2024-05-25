package config

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
)

type ToolConfig struct {
	ToolName    string
	Version     string               `json:"version"`
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

	// Unmarshal the JSON data into the config struct
	err = json.Unmarshal(data, &conf.ToolConfigs)
	if err != nil {
		return
	}
	for toolName, toolConfig := range conf.ToolConfigs {
		toolConfig.ToolName = toolName
	}

	return
}
