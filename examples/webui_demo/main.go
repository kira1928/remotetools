package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kira1928/remotetools/pkg/tools"
)

func main() {
	// tools.AddReadOnlyRootFolder("external_tools")
	// Load configuration
	err := tools.Get().LoadConfig("config/sample.json")
	if err != nil {
		fmt.Println("Failed to load config:", err)
		return
	}

	// Start web UI server on port 8080 (or 0 for random port)
	port := 8080
	err = tools.Get().StartWebUI(port)
	if err != nil {
		fmt.Println("Failed to start web UI:", err)
		return
	}

	actualPort := tools.Get().GetWebUIPort()
	status := tools.Get().GetWebUIStatus()

	fmt.Println("=== Remote Tools Web UI ===")
	fmt.Printf("Status: %s\n", status)
	fmt.Printf("URL: http://localhost:%d\n", actualPort)
	fmt.Println("\nOpen the URL in your browser to manage tools.")
	fmt.Println("Press Ctrl+C to stop the server.")
	fmt.Println()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nStopping web UI server...")
	err = tools.Get().StopWebUI()
	if err != nil {
		fmt.Println("Error stopping server:", err)
	} else {
		fmt.Println("Server stopped successfully")
	}
}
