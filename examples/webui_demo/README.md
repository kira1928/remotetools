# Web UI Management Feature

[English](README.md) | [中文](README_zh.md)

This directory contains an example demonstrating the web UI management feature for Remote Tools.

## Overview

The web UI provides a browser-based interface for managing tool installations. It features:

- **Real-time Installation**: Click to install tools with live progress updates
- **SSE Progress Tracking**: Server-Sent Events for real-time download speed and progress
- **Modern UI**: Responsive design with beautiful gradients and smooth animations
- **Multi-version Support**: View and manage different versions of the same tool
- **Multi-language Support**: Switch between English and Chinese, with preference saved in localStorage
- **Simple Technology**: Pure HTML5/CSS/JS without complex frameworks (total size ~24KB)
- **Template-based DOM**: Uses HTML `<template>` element for safe and efficient DOM manipulation

## Running the Example

```bash
cd /path/to/remotetools
go run examples/webui_demo/main.go
```

Then open your browser to `http://localhost:8080`

## API Usage

```go
package main

import (
    "github.com/kira1928/remotetools/pkg/tools"
)

func main() {
    // Load configuration
    tools.Get().LoadConfig("config/multi_version_sample.json")

    // Start web UI on port 8080 (use 0 for random port)
    err := tools.Get().StartWebUI(8080)
    if err != nil {
        panic(err)
    }

    // Get server information
    port := tools.Get().GetWebUIPort()
    status := tools.Get().GetWebUIStatus()
    
    println("Server running on port:", port)
    println("Status:", status)

    // Later... stop the server
    tools.Get().StopWebUI()
}
```

## Features

### Server Management
- `StartWebUI(port int) error` - Start the web server
- `StopWebUI() error` - Stop the web server
- `GetWebUIStatus() ServerStatus` - Get current status (stopped/starting/running/stopping)
- `GetWebUIPort() int` - Get the port number (0 if not running)

### Progress Tracking
- Real-time download progress with percentage
- Download speed in MB/s
- Status updates (downloading/extracting/completed/failed)
- Error messages displayed in the UI

### Web Interface
- Lists all configured tools with versions
- Shows installation status with color-coded badges
- Install buttons only for non-installed tools
- Live progress bars during installation
- Responsive grid layout
- **Language switcher (English/Chinese) with localStorage persistence**

### Multi-language Support
- Automatically detects browser language on first visit
- Supports English and Chinese
- Language preference saved in localStorage
- Seamless switching without page reload

## API Endpoints

- `GET /` - Main web UI page
- `GET /api/tools` - JSON list of all tools with status
- `POST /api/install` - Start tool installation (JSON body: `{"toolName": "...", "version": "..."}`)
- `GET /api/progress` - SSE endpoint for progress updates

## Architecture

The implementation uses an adapter pattern to avoid import cycles:

1. **pkg/webui** - Web server and HTTP handlers (no dependency on tools package)
   - `server.go` - Server lifecycle management
   - `handlers.go` - HTTP handlers with Go embed for frontend files
   - `frontend/` - Frontend resources (HTML, CSS, JavaScript)
     - `index.html` - Main page structure
     - `style.css` - Styling (~3KB)
     - `app.js` - Application logic (~6KB)
     - `i18n.js` - Internationalization support (~3KB)
2. **pkg/tools/webui_adapter.go** - Adapter implementing the webui.APIAdapter interface
3. **pkg/tools/downloaded_tool.go** - Progress tracking with callback support
4. **pkg/tools/tools.go** - WebUI management methods on the main API

The frontend files are embedded into the Go binary using `//go:embed`, making deployment simple with no external dependencies.

## Screenshots

See the PR description for screenshots of the web UI in action.
