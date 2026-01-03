# Remote Tools - 项目级 AI 规则

## 项目概述

这是一个用于管理和执行远程工具的 Go 库，具有自动版本管理、下载和安装功能。
支持 Web UI 进行可视化管理。

## 目录结构

- `pkg/tools/` - 核心工具管理库代码
- `pkg/webui/` - Web UI 后端 HTTP 处理器
- `pkg/webui/frontend/` - **前端编译产物（已 commit）**
- `web/` - 前端 React + TypeScript 源码
- `examples/` - 示例代码
- `cmd/` - 命令行工具入口
- `config/` - 示例配置文件
- `docs/` - 文档

## 构建命令

```bash
# 使用 Go 构建脚本
go run -tags buildtool ./build.go help
go run -tags buildtool ./build.go build        # 构建当前平台
go run -tags buildtool ./build.go dev          # Debug 构建
go run -tags buildtool ./build.go release      # Release 构建
go run -tags buildtool ./build.go build-all    # 构建全部平台产物
go run -tags buildtool ./build.go test         # 运行测试
go run -tags buildtool ./build.go clean        # 清理

# 或使用 Make（内部调用上述命令）
make build
make test
make release
```

## 前端开发与测试

### 开发模式

```bash
cd web
npm install
npm run dev   # 启动 Vite 开发服务器，端口 5173
```

开发模式下，API 请求会代理到 `http://localhost:8080`。

### 启动后端服务器

**必须先启动后端服务器**，前端才能正常工作。运行示例：

```bash
go run ./examples/webui_demo/main.go
```

这会在 8080 端口启动 Web UI 后端服务。然后可以：
- 直接访问 `http://localhost:8080` 使用生产版前端
- 或使用 `http://localhost:5173` 使用开发版前端（需先运行 `npm run dev`）

### 构建前端

```bash
cd web
npm run build   # 输出到 pkg/webui/frontend/
```

## 前端编译产物 commit 策略

为了让依赖此库的 Go 开发者开箱即用，`pkg/webui/frontend/` 目录下的编译产物会被 commit 到 git。

- 文件名带 content hash（如 `index-abc123.js`），确保浏览器缓存正确失效
- 修改前端代码后，需要运行 `npm run build` 并 commit 新的编译产物
- CI 可以验证编译产物是否与源码匹配

## 代码规范

- 前端使用 TypeScript，严格模式
- 代码注释和 AI 生成的文档使用中文
