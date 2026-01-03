package tools

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"sync/atomic"

	"github.com/google/uuid"
	"github.com/kira1928/remotetools/pkg/config"
	"github.com/kira1928/remotetools/pkg/webui"
	xz "github.com/ulikunitz/xz"
)

// DownloadProgress represents the download progress information
type DownloadProgress struct {
	TotalBytes      int64
	DownloadedBytes int64
	Speed           float64 // bytes per second
	Status          string  // downloading, extracting, completed, failed
	Error           error
	AttemptIndex    int      // 当前尝试的第几个源（1-based）
	TotalAttempts   int      // 总源数
	CurrentURL      string   // 当前下载的 URL
	FailedURLs      []string // 已失败的 URL 列表
	AllURLs         []string // 所有候选 URL（按尝试顺序）
}

// ProgressCallback is called during download to report progress
type ProgressCallback func(progress DownloadProgress)

// MetadataFilenameSuffix 定义工具元数据文件的后缀
const MetadataFilenameSuffix = ".toolmeta.json"

// ToolMetadata 记录工具当前配置、启用状态与下载进度
type ToolMetadata struct {
	DownloadURL     []string        `json:"downloadUrl,omitempty"`
	PathToEntry     string          `json:"pathToEntry,omitempty"`
	PrintInfoCmd    []string        `json:"printInfoCmd,omitempty"`
	DownloadProcess DownloadProcess `json:"downloadProcess"`
}

func loadMetadataFile(path string) (*ToolMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta ToolMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func writeMetadataFile(path string, meta *ToolMetadata) error {
	if meta == nil {
		return fmt.Errorf("metadata is nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}

// DownloadProcess 记录最近一次下载的状态
type DownloadProcess struct {
	CurrentDownloadURLIndex int      `json:"currentDownloadUrlIndex,omitempty"`
	FileSize                int64    `json:"fileSize,omitempty"`
	Status                  string   `json:"status,omitempty"`
	AttemptIndex            int      `json:"attemptIndex,omitempty"`
	TotalAttempts           int      `json:"totalAttempts,omitempty"`
	CurrentURL              string   `json:"currentUrl,omitempty"`
	FailedURLs              []string `json:"failedUrls,omitempty"`
	AllURLs                 []string `json:"allUrls,omitempty"`
}

// DownloadedTool 是带有下载能力的工具实现
type DownloadedTool struct {
	*BaseTool
	group            *ToolGroup // group 用于共享启用状态
	progressCallback ProgressCallback
	paused           int32 // atomic flag: 1 paused, 0 running
	metadataMu       sync.Mutex
	metadata         *ToolMetadata
}

func (p *DownloadedTool) getMetadataPath() string {
	toolFolder := p.GetWritableToolFolder()
	if strings.TrimSpace(toolFolder) == "" {
		return ""
	}
	return filepath.Clean(toolFolder + MetadataFilenameSuffix)
}

func (p *DownloadedTool) loadMetadataFromDisk() (*ToolMetadata, error) {
	path := p.getMetadataPath()
	if path == "" {
		return nil, nil
	}
	meta, err := loadMetadataFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func copyStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func cloneDownloadProcess(src DownloadProcess) DownloadProcess {
	cp := src
	cp.FailedURLs = copyStringSlice(src.FailedURLs)
	cp.AllURLs = copyStringSlice(src.AllURLs)
	return cp
}

// downloadProcessEqual 比较两个下载进度快照是否完全一致。
func downloadProcessEqual(a, b DownloadProcess) bool {
	if a.CurrentDownloadURLIndex != b.CurrentDownloadURLIndex {
		return false
	}
	if a.FileSize != b.FileSize || a.Status != b.Status {
		return false
	}
	if a.AttemptIndex != b.AttemptIndex || a.TotalAttempts != b.TotalAttempts {
		return false
	}
	if a.CurrentURL != b.CurrentURL {
		return false
	}
	if !stringSliceEqual(a.FailedURLs, b.FailedURLs) || !stringSliceEqual(a.AllURLs, b.AllURLs) {
		return false
	}
	return true
}

func (p *DownloadedTool) mergeConfigIntoMetadata(meta *ToolMetadata) bool {
	if meta == nil || p.ToolConfig == nil {
		return false
	}
	changed := false
	desiredURLs := append([]string(nil), p.DownloadURL.Values...)
	if !stringSliceEqual(meta.DownloadURL, desiredURLs) {
		meta.DownloadURL = desiredURLs
		changed = true
	}
	desiredCmd := append([]string(nil), p.PrintInfoCmd...)
	if !stringSliceEqual(meta.PrintInfoCmd, desiredCmd) {
		if len(desiredCmd) == 0 {
			meta.PrintInfoCmd = nil
		} else {
			meta.PrintInfoCmd = desiredCmd
		}
		changed = true
	}
	desiredEntry := p.PathToEntry.Primary()
	if desiredEntry != "" {
		if strings.TrimSpace(meta.PathToEntry) != desiredEntry {
			meta.PathToEntry = desiredEntry
			changed = true
		}
	} else if meta.PathToEntry != "" {
		meta.PathToEntry = ""
		changed = true
	}
	return changed
}

func (p *DownloadedTool) defaultMetadata() *ToolMetadata {
	meta := &ToolMetadata{
		PathToEntry:     "",
		DownloadProcess: DownloadProcess{},
	}
	_ = p.mergeConfigIntoMetadata(meta)
	return meta
}

func (p *DownloadedTool) ensureMetadataLocked() *ToolMetadata {
	if p.metadata != nil {
		return p.metadata
	}
	meta, err := p.loadMetadataFromDisk()
	if err != nil {
		log.Printf("[%s@%s] 读取元数据失败: %v", p.ToolName, p.Version, err)
	}
	needsSave := false
	if meta == nil {
		meta = p.defaultMetadata()
		needsSave = true
	} else if p.mergeConfigIntoMetadata(meta) {
		needsSave = true
	}
	p.metadata = meta
	if needsSave {
		if err := p.writeMetadataLocked(); err != nil {
			log.Printf("[%s@%s] 写入元数据失败: %v", p.ToolName, p.Version, err)
		}
	}
	return meta
}

func (p *DownloadedTool) ensureMetadata() *ToolMetadata {
	p.metadataMu.Lock()
	defer p.metadataMu.Unlock()
	return p.ensureMetadataLocked()
}

func (p *DownloadedTool) writeMetadataLocked() error {
	if p.metadata == nil {
		return nil
	}
	path := p.getMetadataPath()
	if path == "" {
		return fmt.Errorf("metadata path not available for %s@%s", p.ToolName, p.Version)
	}
	return writeMetadataFile(path, p.metadata)
}

func (p *DownloadedTool) updateMetadataFromProgress(dp DownloadProgress) {
	p.metadataMu.Lock()
	defer p.metadataMu.Unlock()
	meta := p.ensureMetadataLocked()
	if meta == nil {
		return
	}
	prev := meta.DownloadProcess
	forcePersist := false
	current := meta.DownloadProcess
	switch dp.Status {
	case "trying":
		idx := dp.AttemptIndex - 1
		if idx < 0 {
			idx = 0
		}
		current = DownloadProcess{
			CurrentDownloadURLIndex: idx,
			Status:                  dp.Status,
			AttemptIndex:            dp.AttemptIndex,
			TotalAttempts:           dp.TotalAttempts,
			CurrentURL:              dp.CurrentURL,
			FailedURLs:              copyStringSlice(dp.FailedURLs),
			AllURLs:                 copyStringSlice(dp.AllURLs),
		}
		if dp.TotalBytes > 0 {
			current.FileSize = dp.TotalBytes
		}
		forcePersist = true
	case "downloading":
		if idx := dp.AttemptIndex - 1; idx >= 0 {
			current.CurrentDownloadURLIndex = idx
		}
		if dp.TotalBytes > 0 {
			current.FileSize = dp.TotalBytes
		}
		current.Status = dp.Status
		current.AttemptIndex = dp.AttemptIndex
		current.TotalAttempts = dp.TotalAttempts
		current.CurrentURL = dp.CurrentURL
		current.FailedURLs = copyStringSlice(dp.FailedURLs)
		current.AllURLs = copyStringSlice(dp.AllURLs)
	case "extracting":
		current.Status = dp.Status
		if dp.TotalBytes > 0 {
			current.FileSize = dp.TotalBytes
		}
		current.AttemptIndex = dp.AttemptIndex
		current.TotalAttempts = dp.TotalAttempts
		current.CurrentURL = dp.CurrentURL
		current.FailedURLs = copyStringSlice(dp.FailedURLs)
		current.AllURLs = copyStringSlice(dp.AllURLs)
		forcePersist = true
	case "paused", "failed":
		if idx := dp.AttemptIndex - 1; idx >= 0 {
			current.CurrentDownloadURLIndex = idx
		}
		if dp.TotalBytes > 0 {
			current.FileSize = dp.TotalBytes
		}
		current.Status = dp.Status
		current.AttemptIndex = dp.AttemptIndex
		current.TotalAttempts = dp.TotalAttempts
		current.CurrentURL = dp.CurrentURL
		current.FailedURLs = copyStringSlice(dp.FailedURLs)
		current.AllURLs = copyStringSlice(dp.AllURLs)
		forcePersist = true
	case "completed":
		current = DownloadProcess{}
		forcePersist = true
	case "disabled":
		current.Status = dp.Status
		current.AttemptIndex = dp.AttemptIndex
		current.TotalAttempts = dp.TotalAttempts
		current.CurrentURL = dp.CurrentURL
		current.FailedURLs = copyStringSlice(dp.FailedURLs)
		current.AllURLs = copyStringSlice(dp.AllURLs)
		forcePersist = true
	default:
		current.Status = dp.Status
		current.AttemptIndex = dp.AttemptIndex
		current.TotalAttempts = dp.TotalAttempts
		current.CurrentURL = dp.CurrentURL
		current.FailedURLs = copyStringSlice(dp.FailedURLs)
		current.AllURLs = copyStringSlice(dp.AllURLs)
	}
	changed := !downloadProcessEqual(prev, current)
	meta.DownloadProcess = current
	if changed || forcePersist {
		if err := p.writeMetadataLocked(); err != nil {
			log.Printf("[%s@%s] 持久化元数据失败: %v", p.ToolName, p.Version, err)
		}
	}
}

func (p *DownloadedTool) resetDownloadProcess(force bool) {
	p.metadataMu.Lock()
	defer p.metadataMu.Unlock()
	meta := p.ensureMetadataLocked()
	if meta == nil {
		return
	}
	meta.DownloadProcess = DownloadProcess{}
	if force {
		if err := p.writeMetadataLocked(); err != nil {
			log.Printf("[%s@%s] 重置元数据下载进度失败: %v", p.ToolName, p.Version, err)
		}
	}
}

func (p *DownloadedTool) SetEnabled(enabled bool) error {
	if p.group == nil {
		return fmt.Errorf("tool group not available for %s", p.ToolName)
	}
	return p.group.SetEnabled(enabled)
}

func (p *DownloadedTool) IsEnabled() bool {
	if p.group == nil {
		return true
	}
	return p.group.IsEnabled()
}

// GetDownloadProcess 返回最近一次下载进度的快照
func (p *DownloadedTool) GetDownloadProcess() DownloadProcess {
	p.metadataMu.Lock()
	defer p.metadataMu.Unlock()
	meta := p.ensureMetadataLocked()
	if meta == nil {
		return DownloadProcess{}
	}
	return cloneDownloadProcess(meta.DownloadProcess)
}

// GetMetadataSnapshot 返回元数据的拷贝，供 UI 展示
func (p *DownloadedTool) GetMetadataSnapshot() *ToolMetadata {
	p.metadataMu.Lock()
	defer p.metadataMu.Unlock()
	meta := p.ensureMetadataLocked()
	if meta == nil {
		return nil
	}
	clone := &ToolMetadata{
		DownloadURL:     copyStringSlice(meta.DownloadURL),
		PathToEntry:     meta.PathToEntry,
		PrintInfoCmd:    copyStringSlice(meta.PrintInfoCmd),
		DownloadProcess: cloneDownloadProcess(meta.DownloadProcess),
	}
	return clone
}

// progressReader wraps an io.Reader to track download progress
type progressReader struct {
	reader          io.Reader
	totalBytes      int64
	downloadedBytes int64
	lastUpdate      time.Time
	lastBytes       int64
	callback        ProgressCallback
	pausedFlag      *int32
	attemptIndex    int
	totalAttempts   int
	currentURL      string
	failedPtr       *[]string
	allURLs         []string
}

func (pr *progressReader) Read(p []byte) (int, error) {
	// honor pause
	if pr.pausedFlag != nil && atomic.LoadInt32(pr.pausedFlag) == 1 {
		return 0, ErrDownloadPaused
	}
	n, err := pr.reader.Read(p)
	pr.downloadedBytes += int64(n)

	now := time.Now()
	if now.Sub(pr.lastUpdate) >= 500*time.Millisecond {
		// Calculate speed
		duration := now.Sub(pr.lastUpdate).Seconds()
		bytesSinceLastUpdate := pr.downloadedBytes - pr.lastBytes
		speed := float64(bytesSinceLastUpdate) / duration

		if pr.callback != nil {
			pr.callback(DownloadProgress{
				TotalBytes:      pr.totalBytes,
				DownloadedBytes: pr.downloadedBytes,
				Speed:           speed,
				Status:          "downloading",
				AttemptIndex:    pr.attemptIndex,
				TotalAttempts:   pr.totalAttempts,
				CurrentURL:      pr.currentURL,
				FailedURLs:      append([]string{}, *pr.failedPtr...),
				AllURLs:         append([]string{}, pr.allURLs...),
			})
		}

		pr.lastUpdate = now
		pr.lastBytes = pr.downloadedBytes
	}

	return n, err
}

var (
	errPaused = errors.New("download paused")
	// ErrDownloadPaused 表示下载任务被手动暂停，可用于区分失败与暂停
	ErrDownloadPaused = errPaused
)

// rateLimitedReader 包装一个 Reader 以限制读取速率（字节/秒）用于测试 UI
type rateLimitedReader struct {
	r       io.Reader
	bps     int64
	started time.Time
	read    int64
}

func newRateLimitedReader(r io.Reader, bps int64) io.Reader {
	if bps <= 0 {
		return r
	}
	return &rateLimitedReader{r: r, bps: bps, started: time.Now()}
}

func (rl *rateLimitedReader) Read(p []byte) (int, error) {
	n, err := rl.r.Read(p)
	rl.read += int64(n)
	if rl.bps > 0 {
		// 期望耗时 = 累计读取字节 / 速率
		expected := time.Duration(float64(rl.read) / float64(rl.bps) * float64(time.Second))
		elapsed := time.Since(rl.started)
		if expected > elapsed {
			time.Sleep(expected - elapsed)
		}
	}
	return n, err
}

func NewDownloadTool(conf *config.ToolConfig, group *ToolGroup) *DownloadedTool {
	tool := &DownloadedTool{
		BaseTool: NewBaseTool(conf),
		group:    group,
	}
	tool.ensureMetadata()
	return tool
}

// SetProgressCallback sets a callback function to receive progress updates
func (p *DownloadedTool) SetProgressCallback(callback ProgressCallback) {
	p.progressCallback = callback
}

// emitProgress 会在有自定义回调时调用回调；否则将消息广播到 WebUI 的 SSE
func (p *DownloadedTool) emitProgress(dp DownloadProgress) {
	p.updateMetadataFromProgress(dp)
	if p.progressCallback != nil {
		p.progressCallback(dp)
		return
	}
	// 回退到全局广播
	msg := webui.ProgressMessage{
		ToolName:        p.ToolName,
		Version:         p.Version,
		Status:          dp.Status,
		TotalBytes:      dp.TotalBytes,
		DownloadedBytes: dp.DownloadedBytes,
		Speed:           dp.Speed,
		AttemptIndex:    dp.AttemptIndex,
		TotalAttempts:   dp.TotalAttempts,
		CurrentURL:      dp.CurrentURL,
		FailedURLs:      dp.FailedURLs,
		AllURLs:         dp.AllURLs,
	}
	if dp.Error != nil {
		msg.Error = dp.Error.Error()
	}
	webui.EmitProgress(msg)
}

func (p *DownloadedTool) Install() error {
	// 对同一工具目录加锁，防止并发安装/卸载/执行等冲突
	tf := p.GetWritableToolFolder()
	mu := getToolMutex(tf)
	if !mu.TryLock() {
		return ErrToolBusy
	}
	defer mu.Unlock()

	// 标记活动任务
	markActive(p.ToolName, p.Version)
	defer unmarkActive(p.ToolName, p.Version)

	if err := p.downloadTool(); err != nil {
		if errors.Is(err, ErrDownloadPaused) {
			// 已经发送 paused 进度，直接返回以避免被视为失败
			return nil
		}
		return err
	}
	// 后置检查：对于可执行程序，安装完成后立即检测执行支持；必要时复制到临时执行目录
	// 注意：此处不发送 completed，由本方法在检查通过后统一发送
	// 仅在工具被标记为可执行时进行
	if p.BaseTool != nil && p.BaseTool.ToolConfig != nil && p.BaseTool.ToolConfig.IsExecutable {
		// 先检查存储目录
		storage := p.GetToolFolder()
		if storage == "" {
			e := errors.New("install succeeded but tool folder not found")
			p.emitProgress(DownloadProgress{Status: "failed", Error: e})
			return e
		}
		if !isExecSupportedCached(storage) {
			// 复制到临时执行目录
			execRoot := GetTmpRootFolderForExecPermission()
			if execRoot != "" {
				execFolder := p.GetToolFolderPath(execRoot)
				// 复制之前，先检测目标目录（临时执行根/execFolder）是否具备执行权限，避免长时间复制后失败
				if !isExecSupportedCached(execFolder) {
					e := fmt.Errorf("tmp exec folder not executable: %s", execFolder)
					p.emitProgress(DownloadProgress{Status: "failed", Error: e})
					return e
				}
				if mkErr := os.MkdirAll(execFolder, 0o755); mkErr == nil {
					// 复制并二次检测
					cpErr := copyDir(storage, execFolder)
					if cpErr != nil {
						// 复制失败时清理并报错
						_ = os.RemoveAll(execFolder)
						p.emitProgress(DownloadProgress{Status: "failed", Error: cpErr})
						return cpErr
					}
					if !isExecSupported(execFolder) {
						// 二次检测失败：清理临时目录
						_ = os.RemoveAll(execFolder)
						e := fmt.Errorf("execution not supported for %s@%s even in tmp exec folder", p.ToolName, p.Version)
						p.emitProgress(DownloadProgress{Status: "failed", Error: e})
						return e
					}
				} else {
					p.emitProgress(DownloadProgress{Status: "failed", Error: mkErr})
					return mkErr
				}
			} else {
				e := errors.New("no tmp exec folder configured for non-executable storage")
				p.emitProgress(DownloadProgress{Status: "failed", Error: e})
				return e
			}
		}
	}
	// 一切正常，发送 completed
	p.emitProgress(DownloadProgress{Status: "completed"})
	return nil
}

func (p *DownloadedTool) GetInstallSource() string {
	return p.getDownloadUrl()
}

func (p *DownloadedTool) getDownloadUrl() string { // 兼容旧调用，返回首个 URL
	return p.DownloadURL.Primary()
}

func (p *DownloadedTool) getDownloadUrls() []string { // 新增：返回全部候选下载链接
	if len(p.DownloadURL.Values) == 0 {
		primary := p.DownloadURL.Primary()
		if primary != "" {
			return []string{primary}
		}
		return nil
	}
	return p.DownloadURL.Values
}

func (p *DownloadedTool) downloadTool() error {
	// 开始/恢复下载前清除暂停标记
	atomic.StoreInt32(&p.paused, 0)
	p.ensureMetadata()
	// 已存在直接完成
	if p.DoesToolExist() {
		p.resetDownloadProcess(true)
		p.emitProgress(DownloadProgress{Status: "completed"})
		return nil
	}

	toolFolder := p.GetWritableToolFolder()
	if _, statErr := os.Stat(toolFolder); os.IsNotExist(statErr) {
		if mkErr := os.MkdirAll(toolFolder, 0755); mkErr != nil {
			p.emitProgress(DownloadProgress{Status: "failed", Error: mkErr})
			return mkErr
		}
	}
	// 清理残留的临时解压目录
	tmpExtractFolder := filepath.Join(filepath.Dir(toolFolder), ".tmp_"+filepath.Base(toolFolder))
	if _, dirErr := os.Stat(tmpExtractFolder); dirErr == nil {
		if rmErr := os.RemoveAll(tmpExtractFolder); rmErr != nil {
			return fmt.Errorf("failed to remove temporary extraction folder: %w", rmErr)
		}
	}

	urls := p.getDownloadUrls()
	if len(urls) == 0 {
		e := errors.New("no download urls provided")
		p.emitProgress(DownloadProgress{Status: "failed", Error: e})
		return e
	}

	failedList := make([]string, 0)
	totalAttempts := len(urls)

	var lastErr error
	for i, u := range urls {
		if u == "" {
			continue
		}
		// 每次尝试前，主动广播一条"切换/开始尝试"消息到 UI（trying 状态）
		p.emitProgress(DownloadProgress{Status: "trying", AttemptIndex: i + 1, TotalAttempts: totalAttempts, CurrentURL: u, FailedURLs: append([]string{}, failedList...), AllURLs: append([]string{}, urls...)})
		if i > 0 {
			log.Printf("[%s@%s] retry with alternative url (%d/%d): %s", p.ToolName, p.Version, i+1, totalAttempts, u)
		} else {
			log.Printf("[%s@%s] start download with url (%d/%d): %s", p.ToolName, p.Version, i+1, totalAttempts, u)
		}
		err := p.attemptDownload(u, toolFolder, i, totalAttempts, urls, &failedList)
		if err != nil {
			if errors.Is(err, ErrDownloadPaused) {
				return ErrDownloadPaused
			}
			lastErr = err
			continue
		}
		// 成功
		return nil
	}
	if lastErr != nil {
		p.emitProgress(DownloadProgress{Status: "failed", Error: lastErr, AttemptIndex: totalAttempts, TotalAttempts: totalAttempts, FailedURLs: append([]string{}, failedList...), AllURLs: append([]string{}, urls...)})
		return lastErr
	}
	return errors.New("unexpected download failure without error")
}

// attemptDownload 尝试从单个 URL 下载工具，返回 nil 表示成功
func (p *DownloadedTool) attemptDownload(url, toolFolder string, idx, totalAttempts int, allURLs []string, failedList *[]string) error {
	// 先发送 HEAD 请求获取文件元信息
	var downloadFileName string
	var knownTotalBytes int64
	if headInfo, herr := getHeadInfo(url); herr == nil {
		if headInfo.FileName != "" {
			downloadFileName = headInfo.FileName
		}
		knownTotalBytes = headInfo.ContentLength
	}
	// 若 HEAD 请求未能获取文件名，回退到从 URL 路径推测
	if downloadFileName == "" {
		name, err := getFileNameFromURL(url)
		if err != nil {
			return err
		}
		downloadFileName = name
	}
	tmpPath := filepath.Join(toolFolder, downloadFileName)

	// 记录本地已存在大小用于断点续传
	var existingSize int64
	if stat, statErr := os.Stat(tmpPath); statErr == nil {
		existingSize = stat.Size()
	}

	// 根据 HEAD 信息和本地文件状态决定下载策略
	needDownload, deleteFirst, err := p.shouldDownload(knownTotalBytes, existingSize, tmpPath, url)
	if err != nil {
		*failedList = append(*failedList, url)
		return err
	}

	// 如果需要删除损坏的本地文件
	if deleteFirst {
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			*failedList = append(*failedList, url)
			return fmt.Errorf("failed to remove corrupted file: %w", err)
		}
		existingSize = 0 // 重置已存在大小
	}

	if needDownload {
		if err := p.performDownload(url, tmpPath, existingSize, knownTotalBytes, idx, totalAttempts, allURLs, failedList); err != nil {
			return err
		}
	}

	// 解压阶段
	return p.extractIfNeeded(downloadFileName, tmpPath, toolFolder, url, idx, totalAttempts, allURLs, failedList)
}

// shouldDownload 检查是否需要下载，返回 (needDownload, deleteFirst, error)
// needDownload: 是否需要执行下载
// deleteFirst: 是否需要先删除本地文件（文件损坏的情况）
func (p *DownloadedTool) shouldDownload(knownTotalBytes, existingSize int64, tmpPath, url string) (bool, bool, error) {
	// 情况1：本地文件不存在，需要下载
	if existingSize == 0 {
		return true, false, nil
	}

	// 情况2：HEAD 未返回文件大小，无法判断，继续下载（断点续传）
	if knownTotalBytes == 0 {
		return true, false, nil
	}

	// 情况3：本地文件大小等于服务器文件大小，已完整下载
	if existingSize == knownTotalBytes {
		log.Printf("[%s@%s] file already complete (%d bytes), skip download", p.ToolName, p.Version, existingSize)
		return false, false, nil
	}

	// 情况4：本地文件大小大于服务器文件大小，文件损坏或服务器文件已变更
	if existingSize > knownTotalBytes {
		log.Printf("[%s@%s] local file (%d bytes) larger than server (%d bytes), will delete and re-download", p.ToolName, p.Version, existingSize, knownTotalBytes)
		return true, true, nil // 需要先删除再下载
	}

	// 情况5：本地文件小于服务器文件，断点续传
	return true, false, nil
}

// performDownload 执行实际的下载操作
func (p *DownloadedTool) performDownload(url, tmpPath string, existingSize, knownTotalBytes int64, idx, totalAttempts int, allURLs []string, failedList *[]string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if existingSize > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingSize))
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		*failedList = append(*failedList, url)
		log.Printf("[%s@%s] attempt %d/%d failed to start: %v, url=%s", p.ToolName, p.Version, idx+1, totalAttempts, err, url)
		return err
	}
	defer resp.Body.Close()

	// 处理 416 Range Not Satisfiable（理论上在 shouldDownload 已处理，但作为防御性编程）
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		if knownTotalBytes > 0 && existingSize == knownTotalBytes {
			// 文件已完整，跳过下载
			return nil
		}
		err := fmt.Errorf("range request failed (416) for %s, local=%d, server=%d", url, existingSize, knownTotalBytes)
		*failedList = append(*failedList, url)
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		err := fmt.Errorf("failed to download tool %s: %s, url: %s", p.ToolName, resp.Status, url)
		*failedList = append(*failedList, url)
		log.Printf("[%s@%s] attempt %d/%d response error: %v", p.ToolName, p.Version, idx+1, totalAttempts, err)
		return err
	}

	// 打开文件写入
	var out *os.File
	if resp.StatusCode == http.StatusPartialContent && existingSize > 0 {
		out, err = os.OpenFile(tmpPath, os.O_WRONLY|os.O_APPEND, 0644)
	} else {
		out, err = os.Create(tmpPath)
		existingSize = 0 // 重新下载，清零已存在大小
	}
	if err != nil {
		return err
	}

	// 计算总大小：优先使用 HEAD 请求获取的大小
	totalBytes := knownTotalBytes
	if totalBytes == 0 {
		totalBytes = resp.ContentLength
		if resp.StatusCode == http.StatusPartialContent && existingSize > 0 {
			totalBytes += existingSize
		}
	}

	// 可选限速（仅用于测试 UI）：通过环境变量 REMOTETOOLS_DOWNLOAD_LIMIT_BPS 启用
	limitedBody := newRateLimitedReader(resp.Body, getDownloadLimitBPS())
	pr := &progressReader{
		reader:          limitedBody,
		totalBytes:      totalBytes,
		downloadedBytes: existingSize,
		lastUpdate:      time.Now(),
		lastBytes:       existingSize,
		callback:        p.emitProgress,
		pausedFlag:      &p.paused,
		attemptIndex:    idx + 1,
		totalAttempts:   totalAttempts,
		currentURL:      url,
		failedPtr:       failedList,
		allURLs:         allURLs,
	}

	if _, err = io.Copy(out, pr); err != nil {
		_ = out.Close()
		if errors.Is(err, ErrDownloadPaused) {
			var currentSize int64
			if stat, sErr := os.Stat(tmpPath); sErr == nil {
				currentSize = stat.Size()
			}
			p.emitProgress(DownloadProgress{
				TotalBytes: totalBytes, DownloadedBytes: currentSize, Speed: 0, Status: "paused",
				AttemptIndex: idx + 1, TotalAttempts: totalAttempts, CurrentURL: url,
				FailedURLs: append([]string{}, *failedList...), AllURLs: append([]string{}, allURLs...),
			})
			return ErrDownloadPaused
		}
		*failedList = append(*failedList, url)
		log.Printf("[%s@%s] attempt %d/%d streaming error: %v", p.ToolName, p.Version, idx+1, totalAttempts, err)
		return err
	}

	if cerr := out.Close(); cerr != nil {
		return cerr
	}
	return nil
}

// extractIfNeeded 根据文件后缀解压文件
func (p *DownloadedTool) extractIfNeeded(downloadFileName, tmpPath, toolFolder, url string, idx, totalAttempts int, allURLs []string, failedList *[]string) error {
	if strings.HasSuffix(downloadFileName, ".zip") || strings.HasSuffix(downloadFileName, ".tar.gz") || strings.HasSuffix(downloadFileName, ".tar.xz") {
		p.emitProgress(DownloadProgress{
			Status: "extracting", AttemptIndex: idx + 1, TotalAttempts: totalAttempts, CurrentURL: url,
			FailedURLs: append([]string{}, *failedList...), AllURLs: append([]string{}, allURLs...),
		})
		if err := extractDownloadedFile(tmpPath, toolFolder); err != nil {
			*failedList = append(*failedList, url)
			return err
		}
		// 删除压缩包
		_ = os.Remove(tmpPath)
	}
	// 非压缩文件直接视为完成
	return nil
}

// Uninstall 移除工具并清空下载进度元数据
func (p *DownloadedTool) Uninstall() error {
	// 先尝试调用 BaseTool.Uninstall()（处理已完成安装的工具）
	baseErr := p.BaseTool.Uninstall()

	// 无论 BaseTool.Uninstall 成功与否，都需要清理可能存在的下载中间文件
	// 因为 GetToolFolder() 只在 entry 存在时返回路径，下载途中的文件需要用 GetWritableToolFolder() 处理
	writableFolder := p.GetWritableToolFolder()
	if writableFolder != "" {
		// 检查可写目录是否存在
		if _, err := os.Stat(writableFolder); err == nil {
			// 目录存在，尝试删除
			parentDir := filepath.Dir(writableFolder)
			trashFolderName := fmt.Sprintf(".trash-%s-%s", filepath.Base(writableFolder), uuid.New().String())
			trashFolder := filepath.Join(parentDir, trashFolderName)

			// 先移动到垃圾目录，再删除
			if err := os.Rename(writableFolder, trashFolder); err == nil {
				_ = os.RemoveAll(trashFolder)
			} else {
				// 如果移动失败，直接尝试删除
				_ = os.RemoveAll(writableFolder)
			}
		}
	}

	// 清理元数据
	metaPath := p.getMetadataPath()
	p.metadataMu.Lock()
	p.metadata = nil
	p.metadataMu.Unlock()
	if metaPath != "" {
		if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		_ = os.Remove(metaPath + ".tmp")
	}

	// 如果 BaseTool.Uninstall 返回了非 nil 错误且不是因为工具不存在，则返回该错误
	if baseErr != nil && baseErr != ErrToolBusy {
		// 如果是因为 toolFolder 为空导致的问题，我们已经处理了，可以忽略
		// 只有真正的锁冲突需要返回错误
		return nil
	}
	return baseErr
}

// GetPartialDownloadInfo returns downloaded size of temp file and last known total size
func (p *DownloadedTool) GetPartialDownloadInfo() (int64, int64, error) {
	metaProcess := p.GetDownloadProcess()
	// 支持多 URL：尝试找到任意一个对应的临时文件
	toolFolder := p.GetWritableToolFolder()
	urls := p.getDownloadUrls()
	var tmpPath string
	for _, u := range urls {
		if u == "" {
			continue
		}
		if name, err := getFileNameFromURL(u); err == nil {
			candidate := filepath.Join(toolFolder, name)
			if _, err := os.Stat(candidate); err == nil {
				tmpPath = candidate
				break
			}
			// 记录首个候选，若都不存在则仍使用第一个用于 HEAD 获取 total
			if tmpPath == "" {
				tmpPath = candidate
			}
		}
	}
	if tmpPath == "" {
		rawURL := p.getDownloadUrl()
		if rawURL != "" {
			if name, err := getFileNameFromURL(rawURL); err == nil {
				tmpPath = filepath.Join(toolFolder, name)
			}
		}
	}
	var existingSize int64
	if tmpPath != "" {
		if stat, err := os.Stat(tmpPath); err == nil {
			existingSize = stat.Size()
		}
	}
	// 优先返回已记录的总大小
	total := metaProcess.FileSize
	if total == 0 {
		// 通过第一个可用 URL 尝试 HEAD
		var headURL string
		if len(urls) > 0 {
			headURL = urls[0]
		} else {
			headURL = p.getDownloadUrl()
		}
		if headURL != "" {
			client := &http.Client{}
			if req, herr := http.NewRequest("HEAD", headURL, nil); herr == nil {
				if resp, herr2 := client.Do(req); herr2 == nil {
					if resp != nil {
						defer resp.Body.Close()
					}
					if resp.StatusCode == http.StatusOK {
						if cl := resp.Header.Get("Content-Length"); cl != "" {
							if s, perr := strconv.ParseInt(cl, 10, 64); perr == nil {
								total = s
							}
						}
					}
				}
			}
		}
	}
	return existingSize, total, nil
}

// Pause signals the current download loop to stop gracefully
func (p *DownloadedTool) Pause() error {
	atomic.StoreInt32(&p.paused, 1)
	return nil
}

// 获取URL中的文件名
func getFileNameFromURL(rawURL string) (string, error) {
	// 解析URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// 提取路径部分
	fileName := path.Base(parsedURL.Path)
	return fileName, nil
}

// headInfo 保存 HEAD 请求返回的元信息
type headInfo struct {
	FileName      string // 从 Content-Disposition 解析的文件名（可能为空）
	ContentLength int64  // 文件总大小（若服务端返回）
}

// getHeadInfo 发送 HEAD 请求获取文件元信息
// 优先从 Content-Disposition 解析文件名，若无则回退到 URL 路径
func getHeadInfo(rawURL string) (*headInfo, error) {
	client := &http.Client{}
	req, err := http.NewRequest("HEAD", rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HEAD request failed with status: %s", resp.Status)
	}

	info := &headInfo{}

	// 尝试从 Content-Length 获取文件大小
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if s, perr := strconv.ParseInt(cl, 10, 64); perr == nil {
			info.ContentLength = s
		}
	}

	// 尝试从 Content-Disposition 解析文件名
	// 格式示例：attachment; filename="example.zip" 或 attachment; filename=example.zip
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		info.FileName = parseContentDispositionFilename(cd)
	}

	return info, nil
}

// parseContentDispositionFilename 从 Content-Disposition header 中解析 filename
func parseContentDispositionFilename(cd string) string {
	// 处理 filename*=UTF-8''encoded 或 filename="name" 或 filename=name
	// 简化实现：依次匹配常见格式

	// 1. 先尝试 filename*=UTF-8''... (RFC 5987)
	if idx := strings.Index(cd, "filename*="); idx >= 0 {
		rest := cd[idx+len("filename*="):]
		// 格式: UTF-8''encoded_filename
		if encIdx := strings.Index(rest, "''"); encIdx >= 0 {
			encoded := rest[encIdx+2:]
			// 截取到分号或结束
			if endIdx := strings.Index(encoded, ";"); endIdx >= 0 {
				encoded = encoded[:endIdx]
			}
			encoded = strings.TrimSpace(encoded)
			if decoded, err := url.PathUnescape(encoded); err == nil && decoded != "" {
				return decoded
			}
		}
	}

	// 2. 尝试 filename="..." (带引号)
	if idx := strings.Index(cd, `filename="`); idx >= 0 {
		start := idx + len(`filename="`)
		rest := cd[start:]
		if endIdx := strings.Index(rest, `"`); endIdx >= 0 {
			return rest[:endIdx]
		}
	}

	// 3. 尝试 filename=... (不带引号)
	if idx := strings.Index(cd, "filename="); idx >= 0 {
		start := idx + len("filename=")
		rest := cd[start:]
		// 截取到分号或结束
		if endIdx := strings.Index(rest, ";"); endIdx >= 0 {
			rest = rest[:endIdx]
		}
		return strings.TrimSpace(rest)
	}

	return ""
}

func extractDownloadedFile(path string, toolFolder string) error {
	// Create temporary extraction folder
	tmpExtractFolder := filepath.Join(filepath.Dir(toolFolder), ".tmp_"+filepath.Base(toolFolder))

	// Remove any existing temporary folder
	if _, err := os.Stat(tmpExtractFolder); err == nil {
		if err := os.RemoveAll(tmpExtractFolder); err != nil {
			return fmt.Errorf("failed to clean up temporary folder: %w", err)
		}
	}

	// Create the temporary folder
	if err := os.MkdirAll(tmpExtractFolder, 0755); err != nil {
		return fmt.Errorf("failed to create temporary extraction folder: %w", err)
	}

	// Extract to temporary folder
	var err error
	if strings.HasSuffix(path, ".zip") {
		err = extractZipFile(path, tmpExtractFolder)
	} else if strings.HasSuffix(path, ".tar.gz") {
		err = extractTarGzFile(path, tmpExtractFolder)
	} else if strings.HasSuffix(path, ".tar.xz") {
		err = extractTarXzFile(path, tmpExtractFolder)
	} else {
		return fmt.Errorf("unsupported file format: %s", path)
	}

	if err != nil {
		// Clean up temporary folder on error
		if rmErr := os.RemoveAll(tmpExtractFolder); rmErr != nil {
			return fmt.Errorf("failed to clean up temporary folder after extraction error: %w", rmErr)
		}
		return err
	}

	// 如果解压后顶层只有一个目录，则视为冗余目录：
	// 直接将该子目录提升为目标目录，避免多一层路径。
	sourceToMove := tmpExtractFolder
	usedSingleDir := false
	if entries, rdErr := os.ReadDir(tmpExtractFolder); rdErr == nil && len(entries) == 1 && entries[0].IsDir() {
		sourceToMove = filepath.Join(tmpExtractFolder, entries[0].Name())
		usedSingleDir = true
	}

	// Atomic move: rename source folder to target folder
	// First remove the target folder if it exists (but it shouldn't since we check DoesToolExist())
	if _, err := os.Stat(toolFolder); err == nil {
		if err := os.RemoveAll(toolFolder); err != nil {
			if rmErr := os.RemoveAll(tmpExtractFolder); rmErr != nil {
				return fmt.Errorf("failed to remove existing target folder: %w; also failed to clean up temp folder: %v", err, rmErr)
			}
			return fmt.Errorf("failed to remove existing target folder: %w", err)
		}
	}

	if err := os.Rename(sourceToMove, toolFolder); err != nil {
		if rmErr := os.RemoveAll(tmpExtractFolder); rmErr != nil {
			return fmt.Errorf("failed to move extracted files to target folder: %w; also failed to clean up temp folder: %v", err, rmErr)
		}
		return fmt.Errorf("failed to move extracted files to target folder: %w", err)
	}

	// 如果提升了单一子目录，tmpExtractFolder 现在应为空，做一次清理
	if usedSingleDir {
		_ = os.RemoveAll(tmpExtractFolder)
	}

	return nil
}

// 解压 zip 文件
func extractZipFile(zipPath string, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if r != nil {
		defer func() {
			if cerr := r.Close(); cerr != nil {
				log.Printf("关闭 zip.Reader 时出错: %v", cerr)
			}
		}()
	}
	if err != nil {
		return err
	}

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if cerr := rc.Close(); cerr != nil {
				log.Printf("关闭 zip 文件时出错: %v", cerr)
			}
		}()

		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			if mkErr := os.MkdirAll(fpath, os.ModePerm); mkErr != nil {
				return mkErr
			}
		} else {
			var dir string
			if lastIndex := strings.LastIndex(fpath, string(os.PathSeparator)); lastIndex > -1 {
				dir = fpath[:lastIndex]
			}
			mkErr := os.MkdirAll(dir, os.ModePerm)
			if mkErr != nil {
				log.Fatal(mkErr)
				return mkErr
			}
			f, err := os.OpenFile(
				fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if f != nil {
				defer func() {
					if cerr := f.Close(); cerr != nil {
						log.Printf("关闭解压后的文件时出错: %v", cerr)
					}
				}()
			}
			if err != nil {
				return err
			}

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func extractTarGzFile(path string, dest string) error {
	// Open the tar.gz file for reading
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			log.Printf("关闭 gz 文件时出错: %v", cerr)
		}
	}()

	// Create a gzip reader
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	// Create a tar reader
	tarReader := tar.NewReader(gzReader)

	// Extract each file from the tar archive
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Determine the file path for the extracted file
		targetPath := filepath.Join(dest, header.Name)

		// Check if the file is a directory
		if header.FileInfo().IsDir() {
			// Create the directory if it doesn't exist
			mkErr := os.MkdirAll(targetPath, header.FileInfo().Mode())
			if mkErr != nil {
				return mkErr
			}
			continue
		}

		// Create the file
		file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.FileInfo().Mode())
		if err != nil {
			return err
		}
		defer file.Close()

		// Copy the contents of the file from the tar archive to the destination file
		_, err = io.Copy(file, tarReader)
		if err != nil {
			return err
		}
	}

	return nil
}

func extractTarXzFile(path string, dest string) error {
	// Open the tar.xz file for reading
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Printf("关闭 xz 文件时出错: %v", cerr)
		}
	}()

	// Create an xz reader
	xzr, err := xz.NewReader(f)
	if err != nil {
		return err
	}

	// Create a tar reader on top of xz reader
	tr := tar.NewReader(xzr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dest, hdr.Name)
		if hdr.FileInfo().IsDir() {
			if mkErr := os.MkdirAll(targetPath, hdr.FileInfo().Mode()); mkErr != nil {
				return mkErr
			}
			continue
		}

		// Ensure parent dir exists
		if mkErr := os.MkdirAll(filepath.Dir(targetPath), 0755); mkErr != nil {
			return mkErr
		}

		out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode())
		if err != nil {
			return err
		}
		if out != nil {
			defer func() {
				if cerr := out.Close(); cerr != nil {
					log.Printf("关闭解压后的文件时出错: %v", cerr)
				}
			}()
		}

		if _, err := io.Copy(out, tr); err != nil {
			return err
		}
	}
	return nil
}
