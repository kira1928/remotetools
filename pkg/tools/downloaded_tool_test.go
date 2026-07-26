package tools

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/kira1928/remotetools/pkg/config"
)

func TestGetCachedDownloadInfoDoesNotAccessNetwork(t *testing.T) {
	previousRoot := GetRootFolder()
	SetRootFolder(t.TempDir())
	t.Cleanup(func() { SetRootFolder(previousRoot) })

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tool := NewDownloadTool(&config.ToolConfig{
		ToolName:    "demo",
		Version:     "v1.0.0",
		DownloadURL: config.OsArchSpecificString{Values: []string{server.URL + "/demo.zip"}},
		PathToEntry: config.OsArchSpecificString{Values: []string{"demo"}},
	}, nil)
	tool.updateMetadataFromProgress(DownloadProgress{Status: "downloading", TotalBytes: 2048})

	toolFolder := tool.GetWritableToolFolder()
	if err := os.MkdirAll(toolFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolFolder, "demo.zip"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}

	downloaded, total, err := tool.GetCachedDownloadInfo()
	if err != nil {
		t.Fatal(err)
	}
	if downloaded != int64(len("partial")) {
		t.Fatalf("downloaded = %d, want %d", downloaded, len("partial"))
	}
	if total != 2048 {
		t.Fatalf("total = %d, want 2048", total)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("状态查询不应访问网络，实际请求次数为 %d", got)
	}
}

func TestGetPartialDownloadInfoFetchesRemoteTotal(t *testing.T) {
	previousRoot := GetRootFolder()
	SetRootFolder(t.TempDir())
	t.Cleanup(func() { SetRootFolder(previousRoot) })

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodHead {
			t.Errorf("method = %s, want HEAD", r.Method)
		}
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tool := NewDownloadTool(&config.ToolConfig{
		ToolName:    "demo-remote-size",
		Version:     "v1.0.0",
		DownloadURL: config.OsArchSpecificString{Values: []string{server.URL + "/demo.zip"}},
		PathToEntry: config.OsArchSpecificString{Values: []string{"demo"}},
	}, nil)

	downloaded, total, err := tool.GetPartialDownloadInfo()
	if err != nil {
		t.Fatal(err)
	}
	if downloaded != 0 {
		t.Fatalf("downloaded = %d, want 0", downloaded)
	}
	if total != 4096 {
		t.Fatalf("total = %d, want 4096", total)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("远端尺寸请求次数 = %d, want 1", got)
	}
}
