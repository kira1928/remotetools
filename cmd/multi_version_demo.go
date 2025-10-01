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

	// 使用 GetToolWithVersion 方法获取指定版本的工具
	fmt.Println("=== 测试多版本支持 ===")
	
	dotnet805, err := tools.Get().GetToolWithVersion("dotnet", "8.0.5")
	if err != nil {
		fmt.Println("获取 dotnet 8.0.5 失败:", err)
	} else {
		fmt.Printf("成功获取 dotnet 8.0.5: 版本=%s, 路径=%s\n", dotnet805.GetVersion(), dotnet805.GetToolPath())
	}
	
	dotnet804, err := tools.Get().GetToolWithVersion("dotnet", "8.0.4")
	if err != nil {
		fmt.Println("获取 dotnet 8.0.4 失败:", err)
	} else {
		fmt.Printf("成功获取 dotnet 8.0.4: 版本=%s, 路径=%s\n", dotnet804.GetVersion(), dotnet804.GetToolPath())
	}
	
	fmt.Println("\n多版本配置测试成功！")
}
