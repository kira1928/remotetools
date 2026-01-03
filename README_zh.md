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

## 构建

本仓库采用跨平台的 Go 构建脚本 `build.go`，Makefile 仅作为入口调用它。

直接使用 Go：

```bash
go run -tags buildtool ./build.go help
go run -tags buildtool ./build.go build        # 构建当前平台
go run -tags buildtool ./build.go dev          # 开发构建（包含调试信息）
go run -tags buildtool ./build.go release      # Release 构建（优化）
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

### 多目录工具查找（容器场景）

工具查找支持"多个只读目录 + 一个读写目录"模式。典型用法：

- 镜像内置若干只读目录，预装常用工具；
- 程序启动后设置一个用户挂载卷作为可写目录，用于后续下载可选工具。

查找顺序：会依次在只读目录列表中查找匹配工具，若未找到，最后在可写目录中查找；若仍未找到且需要安装，将下载并解压到可写目录中。

示例代码：

```go
// 1) 配置只读目录（优先级按顺序）
tools.SetReadOnlyRootFolders([]string{
  "/opt/tools-ro",
  "/usr/local/tools-ro",
})

// 2) 配置可写目录（用于下载/解压/卸载）
tools.SetRootFolder("/data/tools")

// 3) 加载配置并获取工具
api := tools.Get()
_ = api.LoadConfig("config.json")

tool, _ := api.GetTool("mytool")
if !tool.DoesToolExist() {
  _ = tool.Install() // 只会安装到 /data/tools
}
_ = tool.Execute("--version")
```

注意：
- 卸载只针对可写目录执行，不会修改只读目录；
- 如果不设置只读目录，行为与之前完全一致；
- 版本自动选择策略会在所有候选根目录中检测已安装版本（只读优先），以选出"最高已安装版本"。

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

### isExecutable 与执行权限处理（noexec 场景）

- `isExecutable`（布尔，默认 `true`）：指示该条目是否为可直接执行的程序。对于 AnyCPU 的 dll、纯资源包等不可直接运行的条目，请设为 `false`。前端会隐藏"查看信息"按钮并显示"非可执行程序"提示。
- 安装完成后，若 `isExecutable = true`，remotetools 会自动检测存储目录是否支持执行；若不支持且配置了临时执行目录（`SetTmpRootFolderForExecPermission`），会复制到临时目录并再次检测；若仍失败，则安装失败。
- WebUI 会在标题右侧显示当前平台（如 `linux/amd64`），并在工具版本旁显示"临时目录运行"徽标以指示当前从临时目录执行。

更多细节见 docs/IS_EXECUTABLE.md。

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

## 前端开发

Web UI 前端使用 React + TypeScript + Vite 构建。

### 开发模式

```bash
cd web
npm install
npm run dev   # 启动开发服务器，端口 5173
```

API 请求会代理到 `http://localhost:8080`。需要先启动后端：

```bash
go run ./examples/webui_demo/main.go
```

### 构建前端

```bash
cd web
npm run build   # 输出到 pkg/webui/frontend/
```

编译后的前端资源使用带 content hash 的文件名（如 `index-abc123.js`）并 commit 到 git，这样 Go 开发者可以开箱即用，无需自行构建前端。Hash 确保内容变化时浏览器缓存能正确失效。

## 下载限速（测试 UI 用）

为方便观察多镜像下载与进度动画，可通过命令行参数或环境变量临时对单次下载进行限速：

```powershell
# CLI 示例：限制为约 200KB/s
remotetools.exe -download-limit-bps 200000 -tool dotnet -install

# 若更喜欢环境变量方式：
$env:REMOTETOOLS_DOWNLOAD_LIMIT_BPS = "200000"
```

说明：
- 单位为字节/秒（bytes per second）；
- 命令行参数 `-download-limit-bps` 优先级高于环境变量；
- 支持使用下划线或逗号分隔数字：`1_000_000`、`1,000,000`；
- 当参数/环境值为 0 或未设置时视为不限速，仅影响当前进程的后续下载，不会改动配置文件。

## 文档

- [Web UI 文档](examples/webui_demo/README_zh.md) - 详细的 Web UI 指南

## 许可证

详见 [LICENSE](LICENSE) 文件。
