package tools

import "sync"

// 全局活动下载任务（工具名@版本 -> true）
var (
	activeDownloads   = make(map[string]bool)
	activeDownloadsMu sync.RWMutex
)

func markActive(toolName, version string) {
	key := toolName + "@" + version
	activeDownloadsMu.Lock()
	activeDownloads[key] = true
	activeDownloadsMu.Unlock()
}

func unmarkActive(toolName, version string) {
	key := toolName + "@" + version
	activeDownloadsMu.Lock()
	delete(activeDownloads, key)
	activeDownloadsMu.Unlock()
}

func listActiveDownloads() []string {
	activeDownloadsMu.RLock()
	defer activeDownloadsMu.RUnlock()
	res := make([]string, 0, len(activeDownloads))
	for k := range activeDownloads {
		res = append(res, k)
	}
	return res
}
