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

	"github.com/kira1928/remotetools/pkg/config"
)

type DownloadedTool struct {
	*BaseTool
}

func NewDownloadTool(conf *config.ToolConfig) *DownloadedTool {
	return &DownloadedTool{
		BaseTool: NewBaseTool(conf),
	}
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
		return nil
	}

	url := p.getDownloadUrl()

	// download tool using the obtained URL
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// check if the response status code is 200
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download tool %s: %s, url: %s", p.ToolName, resp.Status, url)
	}

	// get the file name from the URL
	downloadFileName, err := getFileNameFromURL(url)
	if err != nil {
		return err
	}

	toolFolder := p.GetToolFolder()

	// Create the directory if it does not exist
	if _, err := os.Stat(toolFolder); os.IsNotExist(err) {
		err = os.MkdirAll(toolFolder, 0755) // You can adjust the file permission as needed
		if err != nil {
			return err
		}
	}

	tmpPath := filepath.Join(toolFolder, downloadFileName)
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	// write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		out.Close()
		return err
	}
	out.Close()

	// 如果下载文件以 .zip 或 .tar.gz 结尾，则解压文件
	if strings.HasSuffix(downloadFileName, ".zip") || strings.HasSuffix(downloadFileName, ".tar.gz") {
		err = extractDownloadedFile(tmpPath)
		if err != nil {
			return err
		}
	}

	// delete the downloaded file
	err = os.Remove(tmpPath)
	if err != nil {
		return err
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

func extractDownloadedFile(path string) error {
	if strings.HasSuffix(path, ".zip") {
		return extractZipFile(path, filepath.Dir(path))
	} else if strings.HasSuffix(path, ".tar.gz") {
		return extractTarGzFile(path)
	} else {
		return fmt.Errorf("unsupported file format: %s", path)
	}
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

func extractTarGzFile(path string) error {
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
		targetPath := filepath.Join(filepath.Dir(path), header.Name)

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
