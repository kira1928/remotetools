---
description: 如何测试前端界面
---

# 测试前端界面

## 前置条件

确保已安装 Node.js 和 npm。

## 步骤

### 1. 启动后端服务器

// turbo
```bash
go run ./examples/webui_demo/main.go
```

后端服务器会在 8080 端口启动，提供 API 和静态资源服务。

### 2. 访问 Web UI

**方式 A：使用生产版前端**

直接访问 `http://localhost:8080`

**方式 B：使用开发版前端（支持热更新）**

// turbo
```bash
cd web
npm install   # 首次需要
npm run dev
```

然后访问 `http://localhost:5173`

开发模式下，API 请求会自动代理到后端的 8080 端口。

## 注意事项

- 后端必须先启动，前端才能正常工作
- 开发模式支持热更新，修改代码后浏览器会自动刷新
- 生产版本需要先运行 `npm run build` 更新 `pkg/webui/frontend/` 目录
