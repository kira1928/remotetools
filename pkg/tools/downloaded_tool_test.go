package tools

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// TestExtractDownloadedFileWithTempFolder tests that extraction uses temporary folder
func TestExtractDownloadedFileWithTempFolder(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	toolFolder := filepath.Join(tempDir, "tool_folder")
	tmpExtractFolder := filepath.Join(tempDir, ".tmp_tool_folder")
	
	// Create a test zip file
	zipPath := filepath.Join(tempDir, "test.zip")
	err := createTestZipFile(zipPath)
	if err != nil {
		t.Fatalf("Failed to create test zip file: %v", err)
	}
	
	// Extract the file
	err = extractDownloadedFile(zipPath, toolFolder)
	if err != nil {
		t.Fatalf("extractDownloadedFile failed: %v", err)
	}
	
	// Verify that the target folder exists
	if _, err := os.Stat(toolFolder); os.IsNotExist(err) {
		t.Errorf("Target folder %s does not exist after extraction", toolFolder)
	}
	
	// Verify that the temporary folder has been cleaned up
	if _, err := os.Stat(tmpExtractFolder); !os.IsNotExist(err) {
		t.Errorf("Temporary folder %s still exists after extraction", tmpExtractFolder)
	}
	
	// Verify that the extracted file exists in the target folder
	extractedFile := filepath.Join(toolFolder, "test.txt")
	if _, err := os.Stat(extractedFile); os.IsNotExist(err) {
		t.Errorf("Extracted file %s does not exist", extractedFile)
	}
}

// TestExtractDownloadedFileCleanupOnError tests that temporary folder is cleaned up on error
func TestExtractDownloadedFileCleanupOnError(t *testing.T) {
	tempDir := t.TempDir()
	toolFolder := filepath.Join(tempDir, "tool_folder")
	tmpExtractFolder := filepath.Join(tempDir, ".tmp_tool_folder")
	
	// Use a non-existent file to cause an error
	nonExistentFile := filepath.Join(tempDir, "nonexistent.zip")
	
	// This should fail and clean up the temporary folder
	err := extractDownloadedFile(nonExistentFile, toolFolder)
	if err == nil {
		t.Fatal("extractDownloadedFile should have failed with non-existent file")
	}
	
	// Verify that the temporary folder has been cleaned up
	if _, err := os.Stat(tmpExtractFolder); !os.IsNotExist(err) {
		t.Errorf("Temporary folder %s should not exist after failed extraction", tmpExtractFolder)
	}
}

// TestExtractDownloadedFileRemovesOldTempFolder tests that old temporary folders are removed
func TestExtractDownloadedFileRemovesOldTempFolder(t *testing.T) {
	tempDir := t.TempDir()
	toolFolder := filepath.Join(tempDir, "tool_folder")
	tmpExtractFolder := filepath.Join(tempDir, ".tmp_tool_folder")
	
	// Create an old temporary folder with some content
	err := os.MkdirAll(tmpExtractFolder, 0755)
	if err != nil {
		t.Fatalf("Failed to create old temp folder: %v", err)
	}
	oldFile := filepath.Join(tmpExtractFolder, "old_file.txt")
	err = os.WriteFile(oldFile, []byte("old content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create old file: %v", err)
	}
	
	// Create a test zip file
	zipPath := filepath.Join(tempDir, "test.zip")
	err = createTestZipFile(zipPath)
	if err != nil {
		t.Fatalf("Failed to create test zip file: %v", err)
	}
	
	// Extract the file - this should remove the old temporary folder
	err = extractDownloadedFile(zipPath, toolFolder)
	if err != nil {
		t.Fatalf("extractDownloadedFile failed: %v", err)
	}
	
	// Verify that the old file is gone
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("Old file %s should have been removed", oldFile)
	}
	
	// Verify that the temporary folder has been cleaned up after successful extraction
	if _, err := os.Stat(tmpExtractFolder); !os.IsNotExist(err) {
		t.Errorf("Temporary folder %s should not exist after extraction", tmpExtractFolder)
	}
}

// TestExtractTarGzFile tests tar.gz extraction
func TestExtractTarGzFile(t *testing.T) {
	tempDir := t.TempDir()
	destFolder := filepath.Join(tempDir, "extracted")
	
	// Create a test tar.gz file
	tarGzPath := filepath.Join(tempDir, "test.tar.gz")
	err := createTestTarGzFile(tarGzPath)
	if err != nil {
		t.Fatalf("Failed to create test tar.gz file: %v", err)
	}
	
	// Create destination folder
	err = os.MkdirAll(destFolder, 0755)
	if err != nil {
		t.Fatalf("Failed to create destination folder: %v", err)
	}
	
	// Extract the file
	err = extractTarGzFile(tarGzPath, destFolder)
	if err != nil {
		t.Fatalf("extractTarGzFile failed: %v", err)
	}
	
	// Verify that the extracted file exists
	extractedFile := filepath.Join(destFolder, "test.txt")
	if _, err := os.Stat(extractedFile); os.IsNotExist(err) {
		t.Errorf("Extracted file %s does not exist", extractedFile)
	}
	
	// Verify file content
	content, err := os.ReadFile(extractedFile)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("Extracted file content = %s; expected 'test content'", string(content))
	}
}

// Helper function to create a test zip file
func createTestZipFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	
	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()
	
	// Add a test file to the zip
	writer, err := zipWriter.Create("test.txt")
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte("test content"))
	return err
}

// Helper function to create a test tar.gz file
func createTestTarGzFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	
	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()
	
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()
	
	// Add a test file to the tar
	content := []byte("test content")
	header := &tar.Header{
		Name: "test.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}
	
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}
	
	if _, err := tarWriter.Write(content); err != nil {
		return err
	}
	
	return nil
}
