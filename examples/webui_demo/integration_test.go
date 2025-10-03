package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/kira1928/remotetools/pkg/tools"
	"github.com/kira1928/remotetools/pkg/webui"
)

// This is an integration test that can be run manually
// go test -v examples/webui_demo/integration_test.go
func TestWebUIIntegration(t *testing.T) {
	// Load configuration
	err := tools.Get().LoadConfig("../../config/sample.json")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test starting web UI with random port
	err = tools.Get().StartWebUI(0)
	if err != nil {
		t.Fatalf("Failed to start web UI: %v", err)
	}

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Verify status
	status := tools.Get().GetWebUIStatus()
	if status != webui.StatusRunning {
		t.Errorf("Expected status to be 'running', got '%s'", status)
	}

	// Verify port
	port := tools.Get().GetWebUIPort()
	if port == 0 {
		t.Error("Expected non-zero port")
	}

	fmt.Printf("Web UI running on port %d\n", port)

	// Test stopping
	err = tools.Get().StopWebUI()
	if err != nil {
		t.Errorf("Failed to stop web UI: %v", err)
	}

	// Wait for server to stop
	time.Sleep(500 * time.Millisecond)

	// Verify stopped
	status = tools.Get().GetWebUIStatus()
	if status != webui.StatusStopped {
		t.Errorf("Expected status to be 'stopped', got '%s'", status)
	}

	port = tools.Get().GetWebUIPort()
	if port != 0 {
		t.Errorf("Expected port to be 0 after stopping, got %d", port)
	}

	fmt.Println("Integration test passed!")
}
