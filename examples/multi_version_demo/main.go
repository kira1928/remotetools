package main

import (
	"fmt"
	"github.com/kira1928/remotetools/pkg/tools"
)

func main() {
	// 加载多版本配置文件
	err := tools.Get().LoadConfig("config/multi_version_sample.json")
	if err != nil {
		fmt.Println("加载配置失败:", err)
		return
	}

	fmt.Println("=== 自动版本选择示例 ===")

	// 示例 1: 使用默认策略 - 优先已安装的最高版本
	fmt.Println("1. 使用 GetTool() - 默认优先已安装版本:")
	dotnet, err := tools.Get().GetTool("dotnet")
	if err != nil {
		fmt.Println("   错误:", err)
	} else {
		fmt.Printf("   自动选择版本: %s (已安装: %v)\n", dotnet.GetVersion(), dotnet.DoesToolExist())
	}

	// 示例 2: 获取最新版本（用于下载）
	fmt.Println("\n2. 使用 GetToolLatest() - 总是选择最新版本:")
	dotnetLatest, err := tools.Get().GetToolLatest("dotnet")
	if err != nil {
		fmt.Println("   错误:", err)
	} else {
		fmt.Printf("   最新版本: %s (已安装: %v)\n", dotnetLatest.GetVersion(), dotnetLatest.DoesToolExist())
		if !dotnetLatest.DoesToolExist() {
			fmt.Println("   可以调用 Install() 下载最新版本")
		}
	}

	// 示例 3: 获取已安装的最高版本（用于执行）
	fmt.Println("\n3. 使用 GetToolInstalled() - 只使用已安装版本:")
	dotnetInstalled, err := tools.Get().GetToolInstalled("dotnet")
	if err != nil {
		fmt.Println("   错误:", err)
	} else {
		fmt.Printf("   已安装的最高版本: %s\n", dotnetInstalled.GetVersion())
	}

	// 示例 4: 指定具体版本
	fmt.Println("\n4. 使用 GetToolWithVersion() - 指定版本:")
	dotnet804, err := tools.Get().GetToolWithVersion("dotnet", "8.0.4")
	if err != nil {
		fmt.Println("   错误:", err)
	} else {
		fmt.Printf("   指定版本: %s (已安装: %v)\n", dotnet804.GetVersion(), dotnet804.DoesToolExist())
	}

	fmt.Println("\n=== 推荐用法 ===")
	fmt.Println("- 下载工具: 使用 GetToolLatest() 获取最新版本，然后调用 Install()")
	fmt.Println("- 执行工具: 使用 GetTool() 或 GetToolInstalled() 获取已安装的版本")
	fmt.Println("- 特定版本: 使用 GetToolWithVersion() 指定版本号")
}
