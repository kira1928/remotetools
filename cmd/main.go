package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/kira1928/remotetools/pkg/tools"
)

var (
	// Global flags
	configPath = flag.String("config", "config/sample.json", "配置文件路径")
	toolFolder = flag.String("tool-folder", "external_tools", "工具存储文件夹路径")
	webui      = flag.Bool("webui", false, "是否启动 WebUI 服务器")
	webuiPort  = flag.Int("webui-port", 8080, "WebUI 服务器端口")

	// Command flags
	listTools   = flag.Bool("list", false, "列出所有工具及其状态")
	checkTool   = flag.String("check", "", "检查指定工具是否存在")
	toolName    = flag.String("tool", "", "指定要使用的工具名称")
	toolVersion = flag.String("version", "", "指定要使用的工具版本号（可选）")
	getVersion  = flag.Bool("get-version", false, "获取指定工具的版本信息")
	getPath     = flag.Bool("get-path", false, "获取指定工具的路径信息")
	install     = flag.Bool("install", false, "安装指定工具")
	uninstall   = flag.Bool("uninstall", false, "卸载指定工具")
	execute     = flag.Bool("exec", false, "执行指定工具")
)

func main() {
	flag.Parse()

	// 设置工具文件夹
	if *toolFolder != "" {
		tools.SetToolFolder(*toolFolder)
	}

	// 加载配置
	if *configPath != "" {
		err := tools.Get().LoadConfig(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载配置文件失败: %v\n", err)
			os.Exit(1)
		}
	}

	// 启动 WebUI 服务器（如果需要）
	if *webui {
		err := tools.Get().StartWebUI(*webuiPort)
		if err != nil {
			fmt.Fprintf(os.Stderr, "启动 WebUI 服务器失败: %v\n", err)
			os.Exit(1)
		}
		port := tools.Get().GetWebUIPort()
		fmt.Printf("WebUI 服务器已启动，端口: %d\n", port)
		fmt.Printf("访问 http://localhost:%d 查看管理界面\n", port)
	}

	// 处理命令
	handled := false

	// 列出所有工具
	if *listTools {
		handleListTools()
		handled = true
	}

	// 检查工具是否存在
	if *checkTool != "" {
		handleCheckTool(*checkTool)
		handled = true
	}

	// 获取工具版本
	if *getVersion {
		if *toolName == "" {
			fmt.Fprintf(os.Stderr, "错误: 请使用 -tool 指定工具名称\n")
			os.Exit(1)
		}
		handleGetVersion(*toolName, *toolVersion)
		handled = true
	}

	// 获取工具路径
	if *getPath {
		if *toolName == "" {
			fmt.Fprintf(os.Stderr, "错误: 请使用 -tool 指定工具名称\n")
			os.Exit(1)
		}
		handleGetPath(*toolName, *toolVersion)
		handled = true
	}

	// 安装工具
	if *install {
		if *toolName == "" {
			fmt.Fprintf(os.Stderr, "错误: 请使用 -tool 指定工具名称\n")
			os.Exit(1)
		}
		handleInstall(*toolName, *toolVersion)
		handled = true
	}

	// 卸载工具
	if *uninstall {
		if *toolName == "" {
			fmt.Fprintf(os.Stderr, "错误: 请使用 -tool 指定工具名称\n")
			os.Exit(1)
		}
		handleUninstall(*toolName, *toolVersion)
		handled = true
	}

	// 执行工具
	if *execute {
		if *toolName == "" {
			fmt.Fprintf(os.Stderr, "错误: 请使用 -tool 指定工具名称\n")
			os.Exit(1)
		}
		args := flag.Args()
		handleExecute(*toolName, *toolVersion, args)
		handled = true
	}

	// 如果没有处理任何命令，显示帮助信息
	if !handled && !*webui {
		printUsage()
		os.Exit(0)
	}

	// 如果启动了 WebUI，等待中断信号
	if *webui {
		fmt.Println("\n按 Ctrl+C 停止服务器...")
		waitForInterrupt()
		fmt.Println("\n正在关闭 WebUI 服务器...")
		tools.Get().StopWebUI()
		fmt.Println("服务器已关闭")
	}
}

func handleListTools() {
	config := tools.Get().GetConfig()
	if config.ToolConfigs == nil {
		fmt.Println("配置中没有工具")
		return
	}

	fmt.Println("工具列表:")
	fmt.Println("----------------------------------------")
	for key, toolConfig := range config.ToolConfigs {
		tool, err := tools.Get().GetToolWithVersion(toolConfig.ToolName, toolConfig.Version)
		status := "未安装"
		if err == nil && tool != nil && tool.DoesToolExist() {
			status = "已安装"
		}
		fmt.Printf("  %s: %s\n", key, status)
		if toolConfig.Version != "" {
			fmt.Printf("    版本: %s\n", toolConfig.Version)
		}
		if tool != nil && tool.DoesToolExist() {
			fmt.Printf("    路径: %s\n", tool.GetToolPath())
		}
	}
}

func handleCheckTool(name string) {
	tool, err := tools.Get().GetTool(name)
	if err != nil {
		fmt.Printf("工具 '%s' 不存在于配置中\n", name)
		os.Exit(1)
	}

	if tool.DoesToolExist() {
		fmt.Printf("工具 '%s' 已安装\n", name)
		fmt.Printf("  版本: %s\n", tool.GetVersion())
		fmt.Printf("  路径: %s\n", tool.GetToolPath())
		os.Exit(0)
	} else {
		fmt.Printf("工具 '%s' 未安装\n", name)
		os.Exit(1)
	}
}

func handleGetVersion(name, version string) {
	tool, err := getTool(name, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取工具失败: %v\n", err)
		os.Exit(1)
	}

	if !tool.DoesToolExist() {
		fmt.Printf("工具 '%s' 未安装\n", name)
		os.Exit(1)
	}

	fmt.Printf("工具版本: %s\n", tool.GetVersion())
}

func handleGetPath(name, version string) {
	tool, err := getTool(name, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取工具失败: %v\n", err)
		os.Exit(1)
	}

	if !tool.DoesToolExist() {
		fmt.Printf("工具 '%s' 未安装\n", name)
		os.Exit(1)
	}

	fmt.Printf("工具路径: %s\n", tool.GetToolPath())
}

func handleInstall(name, version string) {
	tool, err := getTool(name, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取工具失败: %v\n", err)
		os.Exit(1)
	}

	if tool.DoesToolExist() {
		fmt.Printf("工具 '%s' (版本 %s) 已安装\n", name, tool.GetVersion())
		return
	}

	fmt.Printf("正在安装工具 '%s'", name)
	if version != "" {
		fmt.Printf(" (版本 %s)", version)
	}
	fmt.Println("...")

	err = tool.Install()
	if err != nil {
		fmt.Fprintf(os.Stderr, "安装失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("工具 '%s' 安装成功\n", name)
	fmt.Printf("  版本: %s\n", tool.GetVersion())
	fmt.Printf("  路径: %s\n", tool.GetToolPath())
}

func handleUninstall(name, version string) {
	tool, err := getTool(name, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取工具失败: %v\n", err)
		os.Exit(1)
	}

	if !tool.DoesToolExist() {
		fmt.Printf("工具 '%s' 未安装\n", name)
		return
	}

	fmt.Printf("正在卸载工具 '%s' (版本 %s)...\n", name, tool.GetVersion())

	err = tool.Uninstall()
	if err != nil {
		fmt.Fprintf(os.Stderr, "卸载失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("工具 '%s' 卸载成功\n", name)
}

func handleExecute(name, version string, args []string) {
	tool, err := getTool(name, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取工具失败: %v\n", err)
		os.Exit(1)
	}

	if !tool.DoesToolExist() {
		fmt.Printf("工具 '%s' 未安装，正在安装...\n", name)
		err = tool.Install()
		if err != nil {
			fmt.Fprintf(os.Stderr, "安装失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("安装完成\n\n")
	}

	cmd, err := tool.CreateExecuteCmd(args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建执行命令失败: %v\n", err)
		os.Exit(1)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err = cmd.Run()
	if err != nil {
		// 不输出错误信息，让工具自己的错误输出显示
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func getTool(name, version string) (tools.Tool, error) {
	if version != "" {
		return tools.Get().GetToolWithVersion(name, version)
	}
	return tools.Get().GetTool(name)
}

func waitForInterrupt() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
}

func printUsage() {
	fmt.Println("Remote Tools - 远程工具管理器")
	fmt.Println("\n使用方法:")
	fmt.Printf("  %s [选项] [命令]\n\n", os.Args[0])
	fmt.Println("全局选项:")
	fmt.Println("  -config <路径>        配置文件路径 (默认: config/sample.json)")
	fmt.Println("  -tool-folder <路径>   工具存储文件夹 (默认: external_tools)")
	fmt.Println("  -webui                启动 WebUI 服务器")
	fmt.Println("  -webui-port <端口>    WebUI 服务器端口 (默认: 8080)")
	fmt.Println("\n命令:")
	fmt.Println("  -list                 列出所有工具及其状态")
	fmt.Println("  -check <工具名>       检查指定工具是否存在")
	fmt.Println("  -tool <工具名>        指定要操作的工具")
	fmt.Println("  -version <版本>       指定工具版本 (可选)")
	fmt.Println("  -get-version          获取指定工具的版本信息")
	fmt.Println("  -get-path             获取指定工具的路径信息")
	fmt.Println("  -install              安装指定工具")
	fmt.Println("  -uninstall            卸载指定工具")
	fmt.Println("  -exec [参数...]       执行指定工具")
	fmt.Println("\n示例:")
	fmt.Println("  # 列出所有工具")
	fmt.Printf("  %s -list\n\n", os.Args[0])
	fmt.Println("  # 检查工具是否存在")
	fmt.Printf("  %s -check dotnet\n\n", os.Args[0])
	fmt.Println("  # 安装工具")
	fmt.Printf("  %s -tool dotnet -install\n\n", os.Args[0])
	fmt.Println("  # 安装特定版本")
	fmt.Printf("  %s -tool dotnet -version 8.0.5 -install\n\n", os.Args[0])
	fmt.Println("  # 执行工具")
	fmt.Printf("  %s -tool dotnet -exec -- --version\n\n", os.Args[0])
	fmt.Println("  # 启动 WebUI 服务器")
	fmt.Printf("  %s -webui\n\n", os.Args[0])
	fmt.Println("  # 使用自定义配置和 WebUI")
	fmt.Printf("  %s -config myconfig.json -webui -webui-port 9000\n", os.Args[0])
}
