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
	w.Write(data)
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

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Uninstallation completed"))
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
