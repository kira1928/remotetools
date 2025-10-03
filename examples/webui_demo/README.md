# Web UI Management Feature

This directory contains an example demonstrating the web UI management feature for Remote Tools.

## Overview

The web UI provides a browser-based interface for managing tool installations. It features:

- **Real-time Installation**: Click to install tools with live progress updates
- **SSE Progress Tracking**: Server-Sent Events for real-time download speed and progress
- **Modern UI**: Responsive design with beautiful gradients and smooth animations
- **Multi-version Support**: View and manage different versions of the same tool

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
- Install/Reinstall buttons
- Live progress bars during installation
- Responsive grid layout

## API Endpoints

- `GET /` - Main web UI page
- `GET /api/tools` - JSON list of all tools with status
- `POST /api/install` - Start tool installation (JSON body: `{"toolName": "...", "version": "..."}`)
- `GET /api/progress` - SSE endpoint for progress updates

## Architecture

The implementation uses an adapter pattern to avoid import cycles:

1. **pkg/webui** - Web server and HTTP handlers (no dependency on tools package)
2. **pkg/tools/webui_adapter.go** - Adapter implementing the webui.APIAdapter interface
3. **pkg/tools/downloaded_tool.go** - Progress tracking with callback support
4. **pkg/tools/tools.go** - WebUI management methods on the main API

## Screenshots

See the PR description for screenshots of the web UI in action.
