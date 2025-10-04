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
现在仓库采用跨平台的 Go 构建脚本 `build.go`，Makefile 仅作为入口调用它。

直接使用 Go：

```bash
go run -tags buildtool ./build.go help
go run -tags buildtool ./build.go build        # 构建当前平台
go run -tags buildtool ./build.go debug        # Debug 构建
go run -tags buildtool ./build.go release      # Release 构建
go run -tags buildtool ./build.go build-all    # 构建全部平台产物
go run -tags buildtool ./build.go test         # 运行测试
go run -tags buildtool ./build.go clean        # 清理
```

或继续使用 Make（内部调用上述命令）：

```bash
make build
make test
make release
make build-all
```

可选参数：

- GOOS/GOARCH 环境变量可通过 `make build` 透传，也可用 `go run` 方式传入 `-os/-arch`。
  例如：
  - `GOOS=linux GOARCH=amd64 make build`
  - `go run -tags buildtool ./build.go build -os linux -arch amd64`
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
