package main

import (
	"fmt"
	"os"
	"github.com/kira1928/remotetools/pkg/tools"
)

func main() {
	// 加载配置
	err := tools.Get().LoadConfig("config/multi_version_sample.json")
	if err != nil {
		fmt.Println("加载配置失败:", err)
		return
	}

	fmt.Println("=== 实际使用场景演示 ===")
	fmt.Println()

	// 场景 1: 下载最新版本
	fmt.Println("场景 1: 下载最新版本的工具")
	fmt.Println("---------------------------------------")
	dotnetLatest, err := tools.Get().GetToolLatest("dotnet")
	if err != nil {
		fmt.Println("错误:", err)
		return
	}
	fmt.Printf("最新版本: %s\n", dotnetLatest.GetVersion())
	
	if !dotnetLatest.DoesToolExist() {
		fmt.Println("工具未安装，开始下载...")
		err = dotnetLatest.Install()
		if err != nil {
			fmt.Println("下载失败:", err)
			return
		}
		fmt.Println("下载完成！")
	} else {
		fmt.Println("工具已安装")
	}
	fmt.Println()

	// 场景 2: 执行已安装的最高版本
	fmt.Println("场景 2: 执行已安装的工具")
	fmt.Println("---------------------------------------")
	dotnetExec, err := tools.Get().GetToolInstalled("dotnet")
	if err != nil {
		fmt.Println("错误:", err)
		fmt.Println("提示: 需要先安装工具")
		return
	}
	
	fmt.Printf("使用已安装的版本: %s\n", dotnetExec.GetVersion())
	fmt.Println("执行: dotnet --version")
	
	cmd, err := dotnetExec.CreateExecuteCmd("--version")
	if err != nil {
		fmt.Println("错误:", err)
		return
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		fmt.Println("执行失败:", err)
	}
	fmt.Println()

	// 场景 3: 使用默认策略（智能选择）
	fmt.Println("场景 3: 使用默认策略（智能选择）")
	fmt.Println("---------------------------------------")
	dotnet, err := tools.Get().GetTool("dotnet")
	if err != nil {
		fmt.Println("错误:", err)
		return
	}
	
	fmt.Printf("智能选择的版本: %s\n", dotnet.GetVersion())
	if dotnet.DoesToolExist() {
		fmt.Println("该版本已安装，可以直接使用")
	} else {
		fmt.Println("该版本未安装，需要先调用 Install()")
	}
}
