package webui

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
)

// ToolProxy 工具 Web UI 反向代理管理
type ToolProxy struct {
	proxies map[string]*httputil.ReverseProxy
	mu      sync.RWMutex
}

// 全局工具代理实例
var toolProxy = &ToolProxy{
	proxies: make(map[string]*httputil.ReverseProxy),
}

// RegisterToolWebUI 注册工具的 Web UI 代理
// toolName: 工具名称（如 "klive"）
// targetURL: 目标地址（如 "http://localhost:8090"）
func RegisterToolWebUI(toolName, targetURL string) error {
	target, err := url.Parse(targetURL)
	if err != nil {
		return err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// 自定义 Director 处理路径
	origDirector := proxy.Director
	prefix := "/tool/" + toolName
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		// 移除 /tool/<toolName> 前缀
		req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
		req.URL.RawPath = strings.TrimPrefix(req.URL.RawPath, prefix)
		req.Host = target.Host
	}

	toolProxy.mu.Lock()
	toolProxy.proxies[toolName] = proxy
	toolProxy.mu.Unlock()

	return nil
}

// UnregisterToolWebUI 取消注册工具的 Web UI 代理
func UnregisterToolWebUI(toolName string) {
	toolProxy.mu.Lock()
	delete(toolProxy.proxies, toolName)
	toolProxy.mu.Unlock()
}

// GetToolProxy 获取工具的反向代理
func GetToolProxy(toolName string) *httputil.ReverseProxy {
	toolProxy.mu.RLock()
	defer toolProxy.mu.RUnlock()
	return toolProxy.proxies[toolName]
}

// ListRegisteredTools 列出所有注册的工具
func ListRegisteredTools() []string {
	toolProxy.mu.RLock()
	defer toolProxy.mu.RUnlock()
	tools := make([]string, 0, len(toolProxy.proxies))
	for name := range toolProxy.proxies {
		tools = append(tools, name)
	}
	return tools
}

// handleToolProxy 处理工具 Web UI 代理请求
func handleToolProxy(w http.ResponseWriter, r *http.Request) {
	// 从路径中提取工具名
	// 路径格式: /tool/<toolName>/...
	path := strings.TrimPrefix(r.URL.Path, "/tool/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "工具名未指定", http.StatusBadRequest)
		return
	}
	toolName := parts[0]

	proxy := GetToolProxy(toolName)
	if proxy == nil {
		http.Error(w, "工具 "+toolName+" 未注册 Web UI", http.StatusNotFound)
		return
	}

	proxy.ServeHTTP(w, r)
}
