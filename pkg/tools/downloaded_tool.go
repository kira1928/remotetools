package tools

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
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
	"time"

	"sync/atomic"

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
}

// ProgressCallback is called during download to report progress
type ProgressCallback func(progress DownloadProgress)

type DownloadedTool struct {
	*BaseTool
	progressCallback ProgressCallback
	paused           int32 // atomic flag: 1 paused, 0 running
	lastTotalBytes   int64 // last known total bytes
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
}

func (pr *progressReader) Read(p []byte) (int, error) {
	// honor pause
	if pr.pausedFlag != nil && atomic.LoadInt32(pr.pausedFlag) == 1 {
		return 0, errPaused
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
			})
		}

		pr.lastUpdate = now
		pr.lastBytes = pr.downloadedBytes
	}

	return n, err
}

var errPaused = errors.New("paused")

func NewDownloadTool(conf *config.ToolConfig) *DownloadedTool {
	return &DownloadedTool{
		BaseTool: NewBaseTool(conf),
	}
}

// SetProgressCallback sets a callback function to receive progress updates
func (p *DownloadedTool) SetProgressCallback(callback ProgressCallback) {
	p.progressCallback = callback
}

// emitProgress 会在有自定义回调时调用回调；否则将消息广播到 WebUI 的 SSE
func (p *DownloadedTool) emitProgress(dp DownloadProgress) {
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

func (p *DownloadedTool) getDownloadUrl() string {
	return p.DownloadURL.Value
}

func (p *DownloadedTool) downloadTool() error {
	// 开始/恢复下载前清除暂停标记
	atomic.StoreInt32(&p.paused, 0)
	// check if file already exists
	if p.DoesToolExist() {
		p.emitProgress(DownloadProgress{Status: "completed"})
		return nil
	}

	url := p.getDownloadUrl()

	// get the file name from the URL
	downloadFileName, err := getFileNameFromURL(url)
	if err != nil {
		p.emitProgress(DownloadProgress{Status: "failed", Error: err})
		return err
	}

	toolFolder := p.GetWritableToolFolder()

	// Create the directory if it does not exist
	if _, statErr := os.Stat(toolFolder); os.IsNotExist(statErr) {
		mkErr := os.MkdirAll(toolFolder, 0755)
		if mkErr != nil {
			p.emitProgress(DownloadProgress{Status: "failed", Error: mkErr})
			return mkErr
		}
	}

	// Clean up any leftover temporary extraction folders
	tmpExtractFolder := filepath.Join(filepath.Dir(toolFolder), ".tmp_"+filepath.Base(toolFolder))
	if _, dirErr := os.Stat(tmpExtractFolder); dirErr == nil {
		if rmErr := os.RemoveAll(tmpExtractFolder); rmErr != nil {
			return fmt.Errorf("failed to remove temporary extraction folder: %w", rmErr)
		}
	}

	tmpPath := filepath.Join(toolFolder, downloadFileName)

	// Check if partial download exists to support resumable download
	var existingSize int64
	if stat, statErr := os.Stat(tmpPath); statErr == nil {
		existingSize = stat.Size()
	}

	// Create HTTP request with Range header for resumable download
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		p.emitProgress(DownloadProgress{Status: "failed", Error: err})
		return err
	}

	if existingSize > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingSize))
	}

	// download tool using the obtained URL
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		p.emitProgress(DownloadProgress{Status: "failed", Error: err})
		return err
	}
	defer resp.Body.Close()

	// If server returns 416 (Range Not Satisfiable), it might be because our local file size
	// already matches the server file size. Send a HEAD request to verify Content-Length.
	skipDownload := false
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		headReq, herr := http.NewRequest("HEAD", url, nil)
		if herr == nil {
			headResp, herr := client.Do(headReq)
			if herr == nil {
				defer func() {
					if cerr := headResp.Body.Close(); cerr != nil {
						log.Printf("关闭 headResp.Body 时出错: %v", cerr)
					}
				}()
				if headResp.StatusCode == http.StatusOK {
					clHeader := headResp.Header.Get("Content-Length")
					if clHeader != "" {
						if serverSize, perr := strconv.ParseInt(clHeader, 10, 64); perr == nil {
							if existingSize > 0 && existingSize == serverSize {
								// Local file already complete; skip further download and proceed to extraction.
								skipDownload = true
								// fallthrough to extraction path below
							}
						}
					}
				}
			}
		}
	}

	// check if the response status code is 200 (full content) or 206 (partial content)
	if !skipDownload && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		ferr := fmt.Errorf("failed to download tool %s: %s, url: %s", p.ToolName, resp.Status, url)
		p.emitProgress(DownloadProgress{Status: "failed", Error: ferr})
		return ferr
	}

	if !skipDownload {
		// Open file for writing (create or append)
		var out *os.File
		if resp.StatusCode == http.StatusPartialContent && existingSize > 0 {
			// Append to existing file
			out, err = os.OpenFile(tmpPath, os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				p.emitProgress(DownloadProgress{Status: "failed", Error: err})
				return err
			}
		} else {
			// Create new file (server doesn't support resume or no existing file)
			out, err = os.Create(tmpPath)
			if err != nil {
				p.emitProgress(DownloadProgress{Status: "failed", Error: err})
				return err
			}
		}

		// Create progress reader to emit periodic updates (even when started outside WebUI)
		var reader io.Reader = resp.Body
		{
			// Calculate total bytes: if resuming, add existing size to content length
			totalBytes := resp.ContentLength
			if resp.StatusCode == http.StatusPartialContent && existingSize > 0 {
				totalBytes += existingSize
			}
			// store last known total
			p.lastTotalBytes = totalBytes
			pr := &progressReader{
				reader:          resp.Body,
				totalBytes:      totalBytes,
				downloadedBytes: existingSize, // Start from existing size if resuming
				lastUpdate:      time.Now(),
				lastBytes:       existingSize,
				callback:        p.emitProgress,
				pausedFlag:      &p.paused,
			}
			reader = pr
		}

		// write the body to file
		_, err = io.Copy(out, reader)
		if err != nil {
			if cerr := out.Close(); cerr != nil {
				p.emitProgress(DownloadProgress{Status: "failed", Error: cerr})
				return cerr
			}
			// distinguish pause vs real error
			if err == errPaused {
				// 读取当前临时文件大小，作为已下载字节数
				var currentSize int64
				if stat, sErr := os.Stat(tmpPath); sErr == nil {
					currentSize = stat.Size()
				}
				p.emitProgress(DownloadProgress{TotalBytes: p.lastTotalBytes, DownloadedBytes: currentSize, Speed: 0, Status: "paused"})
				return nil
			} else {
				p.emitProgress(DownloadProgress{Status: "failed", Error: err})
				return err
			}
		}
		if cerr := out.Close(); cerr != nil {
			p.emitProgress(DownloadProgress{Status: "failed", Error: cerr})
			return cerr
		}
	}

	// 如果下载文件以 .zip、.tar.gz、.tar.xz 结尾，则解压文件
	if strings.HasSuffix(downloadFileName, ".zip") || strings.HasSuffix(downloadFileName, ".tar.gz") || strings.HasSuffix(downloadFileName, ".tar.xz") {
		p.emitProgress(DownloadProgress{Status: "extracting"})
		err = extractDownloadedFile(tmpPath, toolFolder)
		if err != nil {
			p.emitProgress(DownloadProgress{Status: "failed", Error: err})
			return err
		}
	}

	// delete the downloaded file if exists
	if _, err := os.Stat(tmpPath); err == nil {
		err = os.Remove(tmpPath)
		if err != nil {
			p.emitProgress(DownloadProgress{Status: "failed", Error: err})
			return err
		}
	}

	return nil
}

// GetPartialDownloadInfo returns downloaded size of temp file and last known total size
func (p *DownloadedTool) GetPartialDownloadInfo() (int64, int64, error) {
	rawURL := p.getDownloadUrl()
	downloadFileName, err := getFileNameFromURL(rawURL)
	if err != nil {
		return 0, 0, err
	}
	toolFolder := p.GetWritableToolFolder()
	tmpPath := filepath.Join(toolFolder, downloadFileName)
	var existingSize int64
	if stat, err := os.Stat(tmpPath); err == nil {
		existingSize = stat.Size()
	}
	// 优先使用已记录的总大小；如果尚未获取到（为 0），尝试通过 HEAD 获取
	total := p.lastTotalBytes
	if total == 0 && rawURL != "" {
		client := &http.Client{}
		if req, herr := http.NewRequest("HEAD", rawURL, nil); herr == nil {
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
