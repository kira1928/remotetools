package webui

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// ServerStatus represents the status of the web UI server
type ServerStatus string

const (
	StatusStopped  ServerStatus = "stopped"
	StatusStarting ServerStatus = "starting"
	StatusRunning  ServerStatus = "running"
	StatusStopping ServerStatus = "stopping"
)

// WebUIServer manages the web UI server
type WebUIServer struct {
	server   *http.Server
	port     int
	status   ServerStatus
	mu       sync.RWMutex
	listener net.Listener
}

// NewWebUIServer creates a new WebUIServer instance
func NewWebUIServer() *WebUIServer {
	return &WebUIServer{
		status: StatusStopped,
	}
}

// Start starts the web UI server on the specified port
// If port is 0, a random available port will be chosen
func (s *WebUIServer) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == StatusRunning || s.status == StatusStarting {
		return fmt.Errorf("server is already running or starting")
	}

	s.status = StatusStarting

	// Create listener
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		s.status = StatusStopped
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	s.listener = listener
	s.port = listener.Addr().(*net.TCPAddr).Port

	// Create HTTP server
	mux := http.NewServeMux()
	s.setupRoutes(mux)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		s.mu.Lock()
		s.status = StatusRunning
		s.mu.Unlock()

		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			s.mu.Lock()
			s.status = StatusStopped
			s.mu.Unlock()
		}
	}()

	return nil
}

// Stop stops the web UI server
func (s *WebUIServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != StatusRunning {
		return fmt.Errorf("server is not running")
	}

	s.status = StatusStopping

	if err := s.server.Close(); err != nil {
		s.status = StatusStopped
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	s.status = StatusStopped
	s.port = 0
	return nil
}

// GetStatus returns the current status of the server
func (s *WebUIServer) GetStatus() ServerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// GetPort returns the port the server is running on
// Returns 0 if the server is not running
func (s *WebUIServer) GetPort() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.port
}

func (s *WebUIServer) GetAddresses() (addresses []string, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener == nil {
		return nil, fmt.Errorf("server is not running")
	}

	port := s.port
	addresses = []string{fmt.Sprintf("localhost:%d", port)}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		ua, err := iface.Addrs()
		if err != nil {
			continue // ignore error
		}
		for _, addr := range ua {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			// 仅加入可对外使用的地址：IPv4 或全局单播 IPv6
			if v4 := ip.To4(); v4 != nil {
				key := v4.String()
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				addresses = append(addresses, fmt.Sprintf("http://%s:%d", key, port))
				continue
			}
			// 过滤链路本地 IPv6（如 fe80::），避免不可达/需 zone 的地址
			if !ip.IsGlobalUnicast() {
				continue
			}
			key := ip.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			addresses = append(addresses, fmt.Sprintf("[%s]:%d", key, port))
		}
	}
	return addresses, nil
}
