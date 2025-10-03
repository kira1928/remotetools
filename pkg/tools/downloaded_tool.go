package tools

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/kira1928/remotetools/pkg/config"
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
}

// progressReader wraps an io.Reader to track download progress
type progressReader struct {
	reader           io.Reader
	totalBytes       int64
	downloadedBytes  int64
	lastUpdate       time.Time
	lastBytes        int64
	callback         ProgressCallback
}

func (pr *progressReader) Read(p []byte) (int, error) {
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

func NewDownloadTool(conf *config.ToolConfig) *DownloadedTool {
	return &DownloadedTool{
		BaseTool: NewBaseTool(conf),
	}
}

// SetProgressCallback sets a callback function to receive progress updates
func (p *DownloadedTool) SetProgressCallback(callback ProgressCallback) {
	p.progressCallback = callback
}

func (p *DownloadedTool) Install() error {
	return p.DownloadTool()
}

func (p *DownloadedTool) getDownloadUrl() string {
	return p.DownloadURL.Value
}

func (p *DownloadedTool) DownloadTool() error {
	// check if file already exists
	if p.DoesToolExist() {
		if p.progressCallback != nil {
			p.progressCallback(DownloadProgress{
				Status: "completed",
			})
		}
		return nil
	}

	url := p.getDownloadUrl()

	// get the file name from the URL
	downloadFileName, err := getFileNameFromURL(url)
	if err != nil {
		if p.progressCallback != nil {
			p.progressCallback(DownloadProgress{
				Status: "failed",
				Error:  err,
			})
		}
		return err
	}

	toolFolder := p.GetToolFolder()

	// Create the directory if it does not exist
	if _, err := os.Stat(toolFolder); os.IsNotExist(err) {
		err = os.MkdirAll(toolFolder, 0755)
		if err != nil {
			if p.progressCallback != nil {
				p.progressCallback(DownloadProgress{
					Status: "failed",
					Error:  err,
				})
			}
			return err
		}
	}

	// Clean up any leftover temporary extraction folders
	tmpExtractFolder := filepath.Join(filepath.Dir(toolFolder), ".tmp_"+filepath.Base(toolFolder))
	if _, err := os.Stat(tmpExtractFolder); err == nil {
		os.RemoveAll(tmpExtractFolder)
	}

	tmpPath := filepath.Join(toolFolder, downloadFileName)

	// Check if partial download exists to support resumable download
	var existingSize int64
	if stat, err := os.Stat(tmpPath); err == nil {
		existingSize = stat.Size()
	}

	// Create HTTP request with Range header for resumable download
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		if p.progressCallback != nil {
			p.progressCallback(DownloadProgress{
				Status: "failed",
				Error:  err,
			})
		}
		return err
	}

	if existingSize > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingSize))
	}

	// download tool using the obtained URL
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		if p.progressCallback != nil {
			p.progressCallback(DownloadProgress{
				Status: "failed",
				Error:  err,
			})
		}
		return err
	}
	defer resp.Body.Close()

	// check if the response status code is 200 (full content) or 206 (partial content)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		err := fmt.Errorf("failed to download tool %s: %s, url: %s", p.ToolName, resp.Status, url)
		if p.progressCallback != nil {
			p.progressCallback(DownloadProgress{
				Status: "failed",
				Error:  err,
			})
		}
		return err
	}

	// Open file for writing (create or append)
	var out *os.File
	if resp.StatusCode == http.StatusPartialContent && existingSize > 0 {
		// Append to existing file
		out, err = os.OpenFile(tmpPath, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			if p.progressCallback != nil {
				p.progressCallback(DownloadProgress{
					Status: "failed",
					Error:  err,
				})
			}
			return err
		}
	} else {
		// Create new file (server doesn't support resume or no existing file)
		out, err = os.Create(tmpPath)
		if err != nil {
			if p.progressCallback != nil {
				p.progressCallback(DownloadProgress{
					Status: "failed",
					Error:  err,
				})
			}
			return err
		}
	}

	// Create progress reader if callback is set
	var reader io.Reader = resp.Body
	if p.progressCallback != nil {
		// Calculate total bytes: if resuming, add existing size to content length
		totalBytes := resp.ContentLength
		if resp.StatusCode == http.StatusPartialContent && existingSize > 0 {
			totalBytes += existingSize
		}
		pr := &progressReader{
			reader:           resp.Body,
			totalBytes:       totalBytes,
			downloadedBytes:  existingSize, // Start from existing size if resuming
			lastUpdate:       time.Now(),
			lastBytes:        existingSize,
			callback:         p.progressCallback,
		}
		reader = pr
	}

	// write the body to file
	_, err = io.Copy(out, reader)
	if err != nil {
		out.Close()
		if p.progressCallback != nil {
			p.progressCallback(DownloadProgress{
				Status: "failed",
				Error:  err,
			})
		}
		return err
	}
	out.Close()

	// 如果下载文件以 .zip 或 .tar.gz 结尾，则解压文件
	if strings.HasSuffix(downloadFileName, ".zip") || strings.HasSuffix(downloadFileName, ".tar.gz") {
		if p.progressCallback != nil {
			p.progressCallback(DownloadProgress{
				Status: "extracting",
			})
		}
		err = extractDownloadedFile(tmpPath, toolFolder)
		if err != nil {
			if p.progressCallback != nil {
				p.progressCallback(DownloadProgress{
					Status: "failed",
					Error:  err,
				})
			}
			return err
		}
	}

	// delete the downloaded file
	err = os.Remove(tmpPath)
	if err != nil {
		if p.progressCallback != nil {
			p.progressCallback(DownloadProgress{
				Status: "failed",
				Error:  err,
			})
		}
		return err
	}

	if p.progressCallback != nil {
		p.progressCallback(DownloadProgress{
			Status: "completed",
		})
	}

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
	} else {
		return fmt.Errorf("unsupported file format: %s", path)
	}
	
	if err != nil {
		// Clean up temporary folder on error
		os.RemoveAll(tmpExtractFolder)
		return err
	}
	
	// Atomic move: rename temporary folder to target folder
	// First remove the target folder if it exists (but it shouldn't since we check DoesToolExist())
	if _, err := os.Stat(toolFolder); err == nil {
		if err := os.RemoveAll(toolFolder); err != nil {
			os.RemoveAll(tmpExtractFolder)
			return fmt.Errorf("failed to remove existing target folder: %w", err)
		}
	}
	
	// Atomic rename operation
	if err := os.Rename(tmpExtractFolder, toolFolder); err != nil {
		os.RemoveAll(tmpExtractFolder)
		return fmt.Errorf("failed to move extracted files to target folder: %w", err)
	}
	
	return nil
}

// 解压 zip 文件
func extractZipFile(zipPath string, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
		} else {
			var dir string
			if lastIndex := strings.LastIndex(fpath, string(os.PathSeparator)); lastIndex > -1 {
				dir = fpath[:lastIndex]
			}
			err = os.MkdirAll(dir, os.ModePerm)
			if err != nil {
				log.Fatal(err)
				return err
			}
			f, err := os.OpenFile(
				fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer f.Close()

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
	defer file.Close()

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
			err := os.MkdirAll(targetPath, header.FileInfo().Mode())
			if err != nil {
				return err
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
