package main

import (
	"fmt"
	"os"

	"github.com/kira1928/remotetools/pkg/tools"
)

func main() {
	err := tools.Get().LoadConfig("config/sample.json")
	if err != nil {
		fmt.Println("Failed to load config:", err)
		return
	}
	dotnet, err := tools.Get().GetTool("dotnet")
	if err != nil {
		fmt.Println("Failed to get tool:", err)
		return
	}

	if !dotnet.DoesToolExist() {
		fmt.Println("dotnet not exists, installing...")
		dotnet.Install()
	}
	fmt.Println("dotnet exists")

	cmd, err := dotnet.CreateExecuteCmd(
		`--info`,
	)
	if err != nil {
		fmt.Println("Failed to execute dotnet command:", err)
		return
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		fmt.Println("Failed to execute dotnet command:", err)
		return
	}
}
