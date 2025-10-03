# Remote Tools

[English](README.md) | [中文](README_zh.md)

一个用于管理和执行远程工具的 Go 库，具有自动版本管理、下载和安装功能。

## 功能特性

- **自动工具管理**：根据配置自动下载和安装工具
- **多版本支持**：管理同一工具的多个版本
- **跨平台**：支持不同的操作系统和架构（Windows、Linux、macOS）
- **Web UI**：基于浏览器的工具安装管理界面
- **实时进度**：实时显示下载进度和速度
- **多语言界面**：支持中英文界面

## 快速开始

### 安装

```bash
go get github.com/kira1928/remotetools
```

### 基本使用

```go
package main

import (
    "github.com/kira1928/remotetools/pkg/tools"
)

func main() {
    // 加载配置
    tools.Get().LoadConfig("config/sample.json")
    
    // 获取工具
    dotnet, err := tools.Get().GetTool("dotnet")
    if err != nil {
        panic(err)
    }
    
    // 如果不存在则安装
    if !dotnet.DoesToolExist() {
        dotnet.Install()
    }
    
    // 执行
    dotnet.Execute("--version")
}
```

### Web UI

启动基于 Web 的管理界面：

```go
// 在端口 8080 启动 Web UI
tools.Get().StartWebUI(8080)

// 获取服务器信息
port := tools.Get().GetWebUIPort()
status := tools.Get().GetWebUIStatus()

// 完成后停止
tools.Get().StopWebUI()
```

## 配置

工具使用 JSON 格式配置：

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

## 示例

- [基本使用](examples/usage_scenarios/main.go) - 常见使用模式
- [多版本演示](examples/multi_version_demo/main.go) - 管理多个版本
- [Web UI 演示](examples/webui_demo/main.go) - 基于浏览器的管理界面

## 文档

- [Web UI 文档](examples/webui_demo/README_zh.md) - 详细的 Web UI 指南

## 许可证

详见 [LICENSE](LICENSE) 文件。
