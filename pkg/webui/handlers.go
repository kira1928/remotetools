package webui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
)

//go:embed frontend/*
var frontendFS embed.FS

// ToolInfo represents tool information for the web UI
type ToolInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Installed bool   `json:"installed"`
	// Preinstalled 表示该工具是从只读目录识别到的预装版本
	Preinstalled bool `json:"preinstalled"`
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
	UninstallTool(toolName, version string) error
	// GetDownloadInfo returns partial download information (bytes and total) for a tool version
	GetDownloadInfo(toolName, version string) (int64, int64, error)
	// PauseTool requests pausing current download if in progress
	PauseTool(toolName, version string) error
	// GetToolFolder returns the install folder of a tool version
	GetToolFolder(toolName, version string) (string, error)
	// GetToolInfoString executes configured printInfoCmd and returns stdout
	GetToolInfoString(toolName, version string) (string, error)
	// ListActiveInstalls returns active install keys in the form tool@version
	ListActiveInstalls() []string
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
	// Serve static files
	frontendSubFS, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		panic(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(frontendSubFS))))

	// API routes
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/tools", handleListTools)
	mux.HandleFunc("/api/install", handleInstall)
	mux.HandleFunc("/api/uninstall", handleUninstall)
	mux.HandleFunc("/api/progress", handleSSE)
	mux.HandleFunc("/api/active", handleActiveTasks)
	mux.HandleFunc("/api/pause", handlePause)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/tool-path", handleToolPath)
	mux.HandleFunc("/api/tool-info", handleToolInfo)
}

// handleIndex serves the main HTML page
func handleIndex(w http.ResponseWriter, r *http.Request) {
	// Only serve index.html for root path
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data, err := frontendFS.ReadFile("frontend/index.html")
	if err != nil {
		http.Error(w, "Failed to load page", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(data); err != nil {
		// best-effort write; log or ignore
		return
	}
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
	if err := json.NewEncoder(w).Encode(toolsList); err != nil {
		return
	}
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
	if _, err := w.Write([]byte("Installation started")); err != nil {
		return
	}
}

// handleUninstall handles tool uninstallation requests
func handleUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if apiAdapter == nil {
		http.Error(w, "API not initialized", http.StatusInternalServerError)
		return
	}

	var req InstallRequest // Reuse the same struct
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ToolName == "" || req.Version == "" {
		http.Error(w, "toolName and version are required", http.StatusBadRequest)
		return
	}

	// Perform uninstallation
	err := apiAdapter.UninstallTool(req.ToolName, req.Version)
	if err != nil {
		http.Error(w, fmt.Sprintf("Uninstallation failed: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// 通知前端卸载完成，让其关闭对应的 SSE
	broadcastProgress(ProgressMessage{
		ToolName: req.ToolName,
		Version:  req.Version,
		Status:   "uninstalled",
	})

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Uninstallation completed")); err != nil {
		return
	}
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
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
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

// EmitProgress is an exported helper to broadcast progress updates from other packages (e.g., tools)
func EmitProgress(msg ProgressMessage) {
	broadcastProgress(msg)
}

// ActiveTasksResponse represents whether SSE is needed and the active tasks
type ActiveTasksResponse struct {
	NeedsSSE bool             `json:"needsSSE"`
	Active   []InstallRequest `json:"active"`
}

// handleActiveTasks returns whether there are active install tasks
func handleActiveTasks(w http.ResponseWriter, r *http.Request) {
	// 从工具 API 查询全局活动安装（包含从 Go 代码发起的任务）
	var keys []string
	if apiAdapter != nil {
		keys = apiAdapter.ListActiveInstalls()
	}
	resp := ActiveTasksResponse{
		NeedsSSE: len(keys) > 0,
		Active:   make([]InstallRequest, 0, len(keys)),
	}
	for _, key := range keys {
		// key format: tool@version
		var tool, ver string
		if n := len(key); n > 0 {
			// split only on last '@' to be safe
			for i := n - 1; i >= 0; i-- {
				if key[i] == '@' {
					tool = key[:i]
					ver = key[i+1:]
					break
				}
			}
		}
		if tool != "" && ver != "" {
			resp.Active = append(resp.Active, InstallRequest{ToolName: tool, Version: ver})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handlePause handles pause requests for ongoing downloads
func handlePause(w http.ResponseWriter, r *http.Request) {
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

	if err := apiAdapter.PauseTool(req.ToolName, req.Version); err != nil {
		http.Error(w, "Pause failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := w.Write([]byte("Paused")); err != nil {
		return
	}
}

// ToolRuntimeStatus provides runtime status for a tool on UI
type ToolRuntimeStatus struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	Installed       bool   `json:"installed"`
	Preinstalled    bool   `json:"preinstalled"`
	Downloading     bool   `json:"downloading"`
	Paused          bool   `json:"paused"`
	DownloadedBytes int64  `json:"downloadedBytes"`
	TotalBytes      int64  `json:"totalBytes"`
}

// handleStatus returns runtime status for all tools (installed/downloading/paused and progress)
func handleStatus(w http.ResponseWriter, r *http.Request) {
	if apiAdapter == nil {
		http.Error(w, "API not initialized", http.StatusInternalServerError)
		return
	}

	toolsList, err := apiAdapter.ListTools()
	if err != nil {
		http.Error(w, "Failed to list tools: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Snapshot active installs
	active := make(map[string]bool)
	activeInstallsMu.RLock()
	for k := range activeInstalls {
		active[k] = true
	}
	activeInstallsMu.RUnlock()

	statuses := make([]ToolRuntimeStatus, 0, len(toolsList))
	for _, t := range toolsList {
		key := t.Name + "@" + t.Version
		downloading := active[key]
		downloadedBytes, totalBytes, derr := apiAdapter.GetDownloadInfo(t.Name, t.Version)
		if derr != nil {
			downloadedBytes, totalBytes = 0, 0
		}
		paused := !t.Installed && !downloading && downloadedBytes > 0
		statuses = append(statuses, ToolRuntimeStatus{
			Name:            t.Name,
			Version:         t.Version,
			Installed:       t.Installed,
			Preinstalled:    t.Preinstalled,
			Downloading:     downloading,
			Paused:          paused,
			DownloadedBytes: downloadedBytes,
			TotalBytes:      totalBytes,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(statuses); err != nil {
		return
	}
}

// handleToolPath returns the install folder path for a tool version
func handleToolPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if apiAdapter == nil {
		http.Error(w, "API not initialized", http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()
	toolName := q.Get("toolName")
	version := q.Get("version")
	if toolName == "" || version == "" {
		http.Error(w, "toolName and version are required", http.StatusBadRequest)
		return
	}
	path, err := apiAdapter.GetToolFolder(toolName, version)
	if err != nil {
		http.Error(w, "Failed to get path: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"path": path})
}

// handleToolInfo executes printInfoCmd and returns stdout
func handleToolInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if apiAdapter == nil {
		http.Error(w, "API not initialized", http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()
	toolName := q.Get("toolName")
	version := q.Get("version")
	if toolName == "" || version == "" {
		http.Error(w, "toolName and version are required", http.StatusBadRequest)
		return
	}
	info, err := apiAdapter.GetToolInfoString(toolName, version)
	if err != nil {
		http.Error(w, "Failed to get info: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"info": info})
}
