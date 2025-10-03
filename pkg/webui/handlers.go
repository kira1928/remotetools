package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// ToolInfo represents tool information for the web UI
type ToolInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Installed bool   `json:"installed"`
}

// InstallRequest represents an installation request
type InstallRequest struct {
	ToolName string `json:"toolName"`
	Version  string `json:"version"`
}

// ProgressMessage represents a progress update message for SSE
type ProgressMessage struct {
	ToolName        string  `json:"toolName"`
	Version         string  `json:"version"`
	Status          string  `json:"status"`
	TotalBytes      int64   `json:"totalBytes,omitempty"`
	DownloadedBytes int64   `json:"downloadedBytes,omitempty"`
	Speed           float64 `json:"speed,omitempty"`
	Error           string  `json:"error,omitempty"`
}

// APIAdapter provides methods needed from tools API without import cycle
type APIAdapter interface {
	ListTools() ([]ToolInfo, error)
	InstallTool(toolName, version string, progressCallback func(ProgressMessage)) error
}

var (
	// Global state for SSE clients
	sseClients   = make(map[chan ProgressMessage]bool)
	sseClientsMu sync.RWMutex

	// Active installations
	activeInstalls   = make(map[string]bool)
	activeInstallsMu sync.RWMutex

	// Global API adapter
	apiAdapter APIAdapter
)

// SetAPIAdapter sets the global API adapter
func SetAPIAdapter(adapter APIAdapter) {
	apiAdapter = adapter
}

// setupRoutes configures the HTTP routes
func (s *WebUIServer) setupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/tools", handleListTools)
	mux.HandleFunc("/api/install", handleInstall)
	mux.HandleFunc("/api/progress", handleSSE)
}

// handleIndex serves the main HTML page
func handleIndex(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Remote Tools Manager</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        h1 {
            color: white;
            text-align: center;
            margin-bottom: 30px;
            font-size: 2.5em;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.2);
        }
        .tools-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .tool-card {
            background: white;
            border-radius: 12px;
            padding: 20px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
            transition: transform 0.2s, box-shadow 0.2s;
        }
        .tool-card:hover {
            transform: translateY(-5px);
            box-shadow: 0 8px 12px rgba(0,0,0,0.15);
        }
        .tool-name {
            font-size: 1.5em;
            font-weight: bold;
            color: #333;
            margin-bottom: 10px;
        }
        .tool-version {
            color: #666;
            margin-bottom: 15px;
        }
        .tool-status {
            display: inline-block;
            padding: 5px 12px;
            border-radius: 20px;
            font-size: 0.85em;
            font-weight: 600;
            margin-bottom: 15px;
        }
        .status-installed {
            background: #d4edda;
            color: #155724;
        }
        .status-not-installed {
            background: #f8d7da;
            color: #721c24;
        }
        .status-installing {
            background: #fff3cd;
            color: #856404;
        }
        .install-btn {
            width: 100%;
            padding: 12px;
            border: none;
            border-radius: 6px;
            font-size: 1em;
            font-weight: 600;
            cursor: pointer;
            transition: background-color 0.2s;
        }
        .install-btn:enabled {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
        }
        .install-btn:enabled:hover {
            opacity: 0.9;
        }
        .install-btn:disabled {
            background: #ccc;
            color: #666;
            cursor: not-allowed;
        }
        .progress-container {
            margin-top: 10px;
            display: none;
        }
        .progress-bar {
            width: 100%;
            height: 20px;
            background: #f0f0f0;
            border-radius: 10px;
            overflow: hidden;
            margin-bottom: 5px;
        }
        .progress-fill {
            height: 100%;
            background: linear-gradient(90deg, #667eea 0%, #764ba2 100%);
            transition: width 0.3s;
            width: 0%;
        }
        .progress-text {
            font-size: 0.85em;
            color: #666;
        }
        .error-message {
            color: #dc3545;
            font-size: 0.9em;
            margin-top: 10px;
            display: none;
        }
        .loading {
            text-align: center;
            color: white;
            font-size: 1.2em;
            padding: 40px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🛠️ Remote Tools Manager</h1>
        <div id="tools-container" class="loading">Loading tools...</div>
    </div>

    <script>
        let eventSource = null;
        const toolsData = new Map();

        // Initialize SSE connection
        function initSSE() {
            eventSource = new EventSource('/api/progress');
            
            eventSource.onmessage = function(event) {
                const progress = JSON.parse(event.data);
                updateProgress(progress);
            };

            eventSource.onerror = function() {
                console.error('SSE connection error');
            };
        }

        // Load tools list
        async function loadTools() {
            try {
                const response = await fetch('/api/tools');
                const tools = await response.json();
                
                tools.forEach(tool => {
                    const key = tool.name + '@' + tool.version;
                    toolsData.set(key, tool);
                });
                
                renderTools(tools);
            } catch (error) {
                document.getElementById('tools-container').innerHTML = 
                    '<div class="error-message" style="display:block;">Failed to load tools: ' + error.message + '</div>';
            }
        }

        // Render tools grid
        function renderTools(tools) {
            const container = document.getElementById('tools-container');
            container.className = 'tools-grid';
            container.innerHTML = '';

            tools.forEach(tool => {
                const card = createToolCard(tool);
                container.appendChild(card);
            });
        }

        // Create tool card element
        function createToolCard(tool) {
            const card = document.createElement('div');
            card.className = 'tool-card';
            card.id = 'tool-' + tool.name + '-' + tool.version;

            const statusClass = tool.installed ? 'status-installed' : 'status-not-installed';
            const statusText = tool.installed ? 'Installed' : 'Not Installed';
            const btnText = tool.installed ? 'Reinstall' : 'Install';

            card.innerHTML = '<div class="tool-name">' + tool.name + '</div>' +
                '<div class="tool-version">Version: ' + tool.version + '</div>' +
                '<div class="tool-status ' + statusClass + '">' + statusText + '</div>' +
                '<button class="install-btn" onclick="installTool(\'' + tool.name + '\', \'' + tool.version + '\')">' + btnText + '</button>' +
                '<div class="progress-container">' +
                '    <div class="progress-bar">' +
                '        <div class="progress-fill"></div>' +
                '    </div>' +
                '    <div class="progress-text"></div>' +
                '</div>' +
                '<div class="error-message"></div>';

            return card;
        }

        // Install tool
        async function installTool(toolName, version) {
            const cardId = 'tool-' + toolName + '-' + version;
            const card = document.getElementById(cardId);
            const btn = card.querySelector('.install-btn');
            const progressContainer = card.querySelector('.progress-container');
            const errorMessage = card.querySelector('.error-message');
            const statusDiv = card.querySelector('.tool-status');

            // Reset UI
            btn.disabled = true;
            progressContainer.style.display = 'block';
            errorMessage.style.display = 'none';
            statusDiv.className = 'tool-status status-installing';
            statusDiv.textContent = 'Installing...';

            try {
                const response = await fetch('/api/install', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ toolName, version })
                });

                if (!response.ok) {
                    const error = await response.text();
                    throw new Error(error);
                }
            } catch (error) {
                errorMessage.textContent = 'Error: ' + error.message;
                errorMessage.style.display = 'block';
                btn.disabled = false;
                progressContainer.style.display = 'none';
                statusDiv.className = 'tool-status status-not-installed';
                statusDiv.textContent = 'Installation Failed';
            }
        }

        // Update progress from SSE
        function updateProgress(progress) {
            const cardId = 'tool-' + progress.toolName + '-' + progress.version;
            const card = document.getElementById(cardId);
            if (!card) return;

            const progressContainer = card.querySelector('.progress-container');
            const progressFill = card.querySelector('.progress-fill');
            const progressText = card.querySelector('.progress-text');
            const btn = card.querySelector('.install-btn');
            const statusDiv = card.querySelector('.tool-status');
            const errorMessage = card.querySelector('.error-message');

            switch (progress.status) {
                case 'downloading':
                    progressContainer.style.display = 'block';
                    const percent = progress.totalBytes > 0 
                        ? (progress.downloadedBytes / progress.totalBytes * 100).toFixed(1)
                        : 0;
                    progressFill.style.width = percent + '%';
                    const speedMB = (progress.speed / 1024 / 1024).toFixed(2);
                    progressText.textContent = 'Downloading: ' + percent + '% (' + speedMB + ' MB/s)';
                    break;

                case 'extracting':
                    progressFill.style.width = '100%';
                    progressText.textContent = 'Extracting files...';
                    break;

                case 'completed':
                    progressContainer.style.display = 'none';
                    btn.disabled = false;
                    btn.textContent = 'Reinstall';
                    statusDiv.className = 'tool-status status-installed';
                    statusDiv.textContent = 'Installed';
                    break;

                case 'failed':
                    progressContainer.style.display = 'none';
                    btn.disabled = false;
                    statusDiv.className = 'tool-status status-not-installed';
                    statusDiv.textContent = 'Installation Failed';
                    errorMessage.textContent = 'Error: ' + (progress.error || 'Unknown error');
                    errorMessage.style.display = 'block';
                    break;
            }
        }

        // Initialize
        window.onload = function() {
            initSSE();
            loadTools();
        };

        // Cleanup on page unload
        window.onbeforeunload = function() {
            if (eventSource) {
                eventSource.close();
            }
        };
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// handleListTools returns a list of all tools from config
func handleListTools(w http.ResponseWriter, r *http.Request) {
	if apiAdapter == nil {
		http.Error(w, "API not initialized", http.StatusInternalServerError)
		return
	}

	toolsList, err := apiAdapter.ListTools()
	if err != nil {
		http.Error(w, "Failed to list tools: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toolsList)
}

// handleInstall handles tool installation requests
func handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if apiAdapter == nil {
		http.Error(w, "API not initialized", http.StatusInternalServerError)
		return
	}

	var req InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ToolName == "" || req.Version == "" {
		http.Error(w, "toolName and version are required", http.StatusBadRequest)
		return
	}

	// Check if already installing
	installKey := req.ToolName + "@" + req.Version
	activeInstallsMu.Lock()
	if activeInstalls[installKey] {
		activeInstallsMu.Unlock()
		http.Error(w, "Installation already in progress", http.StatusConflict)
		return
	}
	activeInstalls[installKey] = true
	activeInstallsMu.Unlock()

	// Start installation in background
	go func() {
		defer func() {
			activeInstallsMu.Lock()
			delete(activeInstalls, installKey)
			activeInstallsMu.Unlock()
		}()

		err := apiAdapter.InstallTool(req.ToolName, req.Version, func(msg ProgressMessage) {
			broadcastProgress(msg)
		})

		if err != nil {
			broadcastProgress(ProgressMessage{
				ToolName: req.ToolName,
				Version:  req.Version,
				Status:   "failed",
				Error:    err.Error(),
			})
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Installation started"))
}

// handleSSE handles Server-Sent Events for progress updates
func handleSSE(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create client channel
	clientChan := make(chan ProgressMessage, 10)

	// Register client
	sseClientsMu.Lock()
	sseClients[clientChan] = true
	sseClientsMu.Unlock()

	// Remove client on disconnect
	defer func() {
		sseClientsMu.Lock()
		delete(sseClients, clientChan)
		sseClientsMu.Unlock()
		close(clientChan)
	}()

	// Send progress updates
	for {
		select {
		case msg := <-clientChan:
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

// broadcastProgress sends progress updates to all connected SSE clients
func broadcastProgress(msg ProgressMessage) {
	sseClientsMu.RLock()
	defer sseClientsMu.RUnlock()

	for clientChan := range sseClients {
		select {
		case clientChan <- msg:
		default:
			// Client channel is full, skip
		}
	}
}
