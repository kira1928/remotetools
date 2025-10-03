# Remote Tools

[English](README.md) | [中文](README_zh.md)

A Go library for managing and executing remote tools with automatic version management, download, and installation capabilities.

## Features

- **Automatic Tool Management**: Download and install tools automatically based on configuration
- **Multi-version Support**: Manage multiple versions of the same tool
- **Cross-platform**: Support for different OS and architectures (Windows, Linux, macOS)
- **Web UI**: Browser-based interface for managing tool installations
- **Real-time Progress**: Live download progress with speed indicators
- **Multi-language UI**: English and Chinese interface support

## Quick Start

### Installation

```bash
go get github.com/kira1928/remotetools
```

### Basic Usage

```go
package main

import (
    "github.com/kira1928/remotetools/pkg/tools"
)

func main() {
    // Load configuration
    tools.Get().LoadConfig("config/sample.json")
    
    // Get a tool
    dotnet, err := tools.Get().GetTool("dotnet")
    if err != nil {
        panic(err)
    }
    
    // Install if not exists
    if !dotnet.DoesToolExist() {
        dotnet.Install()
    }
    
    // Execute
    dotnet.Execute("--version")
}
```

### Web UI

Start the web-based management interface:

```go
// Start web UI on port 8080
tools.Get().StartWebUI(8080)

// Get server info
port := tools.Get().GetWebUIPort()
status := tools.Get().GetWebUIStatus()

// Stop when done
tools.Get().StopWebUI()
```

## Configuration

Tools are configured in JSON format:

```json
{
  "dotnet": {
    "8.0.5": {
      "downloadUrl": {
        "windows": {
          "amd64": "https://download.visualstudio.microsoft.com/..."
        },
        "linux": {
          "amd64": "https://download.visualstudio.microsoft.com/..."
        }
      },
      "pathToEntry": {
        "windows": "dotnet.exe",
        "linux": "dotnet"
      }
    }
  }
}
```

## Examples

- [Basic Usage](examples/usage_scenarios/main.go) - Common usage patterns
- [Multi-version Demo](examples/multi_version_demo/main.go) - Managing multiple versions
- [Web UI Demo](examples/webui_demo/main.go) - Browser-based management interface

## Documentation

- [Web UI Documentation](examples/webui_demo/README.md) - Detailed Web UI guide

## License

See [LICENSE](LICENSE) file for details.
