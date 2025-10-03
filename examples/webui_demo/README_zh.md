# Web UI 管理功能

[English](README.md) | [中文](README_zh.md)

此目录包含演示 Remote Tools 的 Web UI 管理功能的示例。

## 概述

Web UI 提供基于浏览器的界面来管理工具安装。它具有以下特性：

- **实时安装**：点击安装工具，实时显示进度更新
- **SSE 进度跟踪**：服务器发送事件（SSE）实时显示下载速度和进度
- **现代化界面**：响应式设计，美观的渐变色和流畅的动画效果
- **多版本支持**：查看和管理同一工具的不同版本
- **多语言支持**：中英文界面切换，偏好设置保存在 localStorage
- **简单技术栈**：纯 HTML5/CSS/JS，无复杂框架（总大小约 24KB）
- **基于模板的 DOM**：使用 HTML `<template>` 元素进行安全高效的 DOM 操作

## 运行示例

```bash
cd /path/to/remotetools
go run examples/webui_demo/main.go
```

然后在浏览器中打开 `http://localhost:8080`

## API 使用

```go
package main

import (
    "github.com/kira1928/remotetools/pkg/tools"
)

func main() {
    // 加载配置
    tools.Get().LoadConfig("config/sample.json")

    // 启动 Web UI，端口 8080（使用 0 表示随机端口）
    err := tools.Get().StartWebUI(8080)
    if err != nil {
        panic(err)
    }

    // 获取服务器信息
    port := tools.Get().GetWebUIPort()
    status := tools.Get().GetWebUIStatus()
    
    println("服务器运行在端口:", port)
    println("状态:", status)

    // 稍后...停止服务器
    tools.Get().StopWebUI()
}
```

## 功能特性

### 服务器管理
- `StartWebUI(port int) error` - 启动 Web 服务器
- `StopWebUI() error` - 停止 Web 服务器
- `GetWebUIStatus() ServerStatus` - 获取当前状态（stopped/starting/running/stopping）
- `GetWebUIPort() int` - 获取端口号（未运行时返回 0）

### 进度跟踪
- 实时下载进度百分比
- 下载速度（MB/s）
- 状态更新（下载中/解压中/完成/失败）
- 在界面中显示错误消息

### Web 界面
- 列出所有配置的工具及其版本
- 使用颜色编码的徽章显示安装状态
- 仅为未安装的工具显示安装按钮
- 安装过程中显示实时进度条
- 响应式网格布局
- **语言切换器（中英文），偏好设置保存在 localStorage**

### 多语言支持
- 首次访问时自动检测浏览器语言
- 支持英文和中文
- 语言偏好保存在 localStorage
- 无需刷新页面即可切换语言

## API 端点

- `GET /` - Web UI 主页
- `GET /api/tools` - 返回所有工具及其状态的 JSON 列表
- `POST /api/install` - 启动工具安装（JSON 请求体：`{"toolName": "...", "version": "..."}`）
- `GET /api/progress` - SSE 端点，用于进度更新

## 架构

该实现使用适配器模式避免导入循环：

1. **pkg/webui** - Web 服务器和 HTTP 处理器（不依赖 tools 包）
   - `server.go` - 服务器生命周期管理
   - `handlers.go` - HTTP 处理器，使用 Go embed 嵌入前端文件
   - `frontend/` - 前端资源（HTML、CSS、JavaScript）
     - `index.html` - 主页面结构
     - `style.css` - 样式定义（约 3KB）
     - `app.js` - 应用逻辑（约 6KB）
     - `i18n.js` - 国际化支持（约 3KB）
2. **pkg/tools/webui_adapter.go** - 实现 webui.APIAdapter 接口的适配器
3. **pkg/tools/downloaded_tool.go** - 支持回调的进度跟踪
4. **pkg/tools/tools.go** - 主 API 上的 WebUI 管理方法

前端文件使用 `//go:embed` 嵌入到 Go 二进制文件中，使部署变得简单，无需外部依赖。

## 截图

请参见 PR 描述中的 Web UI 运行截图。
