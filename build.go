//go:build buildtool

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// 一个简单的、可跨平台的构建脚本，复刻原 Makefile 的核心目标：
// build、dev、release、build-all、install、test、clean、run、help
// 用法：
//   go run ./build.go [task] [flags]
// 例如：
//   go run ./build.go build
//   go run ./build.go release
//   go run ./build.go build-all
//   go run ./build.go build -os linux -arch amd64

const (
	defaultAppName  = "remotetools"
	defaultBuildDir = "build"
	defaultDistDir  = "dist"
)

// 与原 Makefile 对齐的平台列表
var allPlatforms = []string{
	"darwin/amd64",
	"darwin/arm64",
	"linux/amd64",
	"linux/arm64",
	"linux/386",
	"windows/amd64",
	"windows/386",
	"windows/arm64",
}

type options struct {
	appName  string
	buildDir string
	distDir  string
	goos     string
	goarch   string
	verbose  bool
}

func main() {
	// 解析子命令与 flags（尽量保持简单，避免引入第三方依赖）
	task, opts := parseArgs(os.Args[1:])
	if task == "help" || task == "-h" || task == "--help" || task == "" {
		printHelp()
		return
	}

	// 确保输出目录存在
	mustMkdirAll(opts.buildDir)
	mustMkdirAll(opts.distDir)

	// 分发任务
	var err error
	switch task {
	case "build":
		err = buildCurrent(opts, "")
	case "dev":
		err = buildCurrent(opts, "dev")
	case "release":
		err = buildCurrent(opts, "release")
	case "build-all":
		err = buildAll(opts)
	case "install":
		err = goInstall(opts)
	case "test":
		err = goTest(opts)
	case "clean":
		err = clean(opts)
	case "run":
		err = runBuiltBinary(opts)
	default:
		printHelp()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (string, options) {
	// 默认值
	opts := options{
		appName:  defaultAppName,
		buildDir: defaultBuildDir,
		distDir:  defaultDistDir,
		goos:     runtime.GOOS,
		goarch:   runtime.GOARCH,
		verbose:  false,
	}

	task := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		task = args[0]
		args = args[1:]
	}

	// 简单 flag 解析
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-os":
			i++
			if i < len(args) {
				opts.goos = args[i]
			}
		case "-arch":
			i++
			if i < len(args) {
				opts.goarch = args[i]
			}
		case "-app-name":
			i++
			if i < len(args) {
				opts.appName = args[i]
			}
		case "-build-dir":
			i++
			if i < len(args) {
				opts.buildDir = args[i]
			}
		case "-dist-dir":
			i++
			if i < len(args) {
				opts.distDir = args[i]
			}
		case "-v", "-verbose":
			opts.verbose = true
		case "-h", "--help", "help":
			task = "help"
		default:
			// 忽略未知参数，或将来扩展
		}
	}
	return task, opts
}

func printHelp() {
	fmt.Println("Remote Tools 构建系统 (Go 版)")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  go run ./build.go <task> [flags]")
	fmt.Println()
	fmt.Println("任务:")
	fmt.Println("  build         构建当前平台")
	fmt.Println("  dev           构建 debug 版本")
	fmt.Println("  release       构建 release 版本")
	fmt.Println("  build-all     构建所有平台")
	fmt.Println("  install       安装到 GOPATH/bin 或 GOBIN")
	fmt.Println("  test          运行测试")
	fmt.Println("  clean         清理 build/ 与 dist/")
	fmt.Println("  run           先构建再运行")
	fmt.Println("  help          显示此帮助信息")
	fmt.Println()
	fmt.Println("常用参数:")
	fmt.Println("  -os <GOOS>           目标操作系统 (默认: 当前)")
	fmt.Println("  -arch <GOARCH>       目标架构 (默认: 当前)")
	fmt.Println("  -app-name <name>     可执行文件名称 (默认: remotetools)")
	fmt.Println("  -build-dir <dir>     构建输出目录 (默认: build)")
	fmt.Println("  -dist-dir <dir>      多平台打包目录 (默认: dist)")
}

func buildCurrent(opts options, mode string) error {
	fmt.Printf("构建 %s/%s (%s) ...\n", opts.goos, opts.goarch, modeOrDefault(mode))

	outName := opts.appName
	if mode == "dev" {
		outName += "-debug"
	}
	if opts.goos == "windows" {
		outName += ".exe"
	}
	outPath := filepath.Join(opts.buildDir, outName)

	args := []string{"build"}
	// ldflags：避免引用不存在的 -X 变量导致链接失败，仅在 release 时做瘦身
	switch mode {
	case "dev":
		// 关闭内联与优化方便调试
		args = append(args, "-gcflags", "all=-N -l")
	case "release":
		args = append(args, "-ldflags", "-s -w")
	}

	// 源入口
	args = append(args, "-o", outPath, "./cmd/main.go")

	env := os.Environ()
	env = append(env, "GOOS="+opts.goos, "GOARCH="+opts.goarch)

	if err := runCmd("go", args, env, opts.verbose); err != nil {
		return err
	}

	// 仅当宿主与目标同为非 Windows 时尝试 strip（避免交叉产物被错误 strip）
	if mode == "release" && runtime.GOOS == opts.goos && opts.goos != "windows" {
		if stripPath, _ := exec.LookPath("strip"); stripPath != "" {
			_ = runCmd(stripPath, []string{outPath}, env, opts.verbose)
		}
	}

	fmt.Printf("构建完成: %s\n", outPath)
	return nil
}

func buildAll(opts options) error {
	fmt.Println("开始构建所有平台 ...")
	for _, p := range allPlatforms {
		parts := strings.SplitN(p, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("无效平台: %s", p)
		}
		osName, arch := parts[0], parts[1]

		outName := fmt.Sprintf("%s-%s-%s", opts.appName, osName, arch)
		if osName == "windows" {
			outName += ".exe"
		}
		outPath := filepath.Join(opts.distDir, outName)

		args := []string{"build", "-ldflags", "-s -w", "-o", outPath, "./cmd/main.go"}
		env := os.Environ()
		env = append(env, "GOOS="+osName, "GOARCH="+arch)

		fmt.Printf("构建 %s/%s ...\n", osName, arch)
		if err := runCmd("go", args, env, opts.verbose); err != nil {
			return err
		}

		if runtime.GOOS == osName && osName != "windows" {
			if stripPath, _ := exec.LookPath("strip"); stripPath != "" {
				_ = runCmd(stripPath, []string{outPath}, env, opts.verbose)
			}
		}
	}
	fmt.Printf("所有平台构建完成，产物位于 %s/\n", opts.distDir)
	return nil
}

func goInstall(opts options) error {
	fmt.Println("安装到 GOPATH/bin 或 GOBIN ...")
	return runCmd("go", []string{"install", "./cmd/main.go"}, os.Environ(), opts.verbose)
}

func goTest(opts options) error {
	fmt.Println("运行测试 ...")
	return runCmd("go", []string{"test", "-v", "./..."}, os.Environ(), opts.verbose)
}

func clean(opts options) error {
	fmt.Println("清理构建文件 ...")
	if err := os.RemoveAll(opts.buildDir); err != nil {
		return err
	}
	if err := os.RemoveAll(opts.distDir); err != nil {
		return err
	}
	return nil
}

func runBuiltBinary(opts options) error {
	// 如果还没构建，先构建
	if err := buildCurrent(opts, ""); err != nil {
		return err
	}
	bin := opts.appName
	if opts.goos == "windows" {
		bin += ".exe"
	}
	binPath := filepath.Join(opts.buildDir, bin)
	fmt.Printf("运行: %s\n\n", binPath)
	return runCmd(binPath, []string{}, os.Environ(), true)
}

func runCmd(cmd string, args []string, env []string, verbose bool) error {
	if verbose {
		fmt.Printf("$ %s %s\n", cmd, strings.Join(args, " "))
	}
	c := exec.Command(cmd, args...)
	if env != nil {
		c.Env = env
	}
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}

func mustMkdirAll(p string) {
	if p == "" {
		return
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "创建目录失败:", p, err)
		os.Exit(1)
	}
}

// 可选：读取 git 与 go 版本信息（当前未注入 -X，以免变量缺失链接失败）
func readVersionInfo() (version, buildTime, goVersion string) {
	version = readGitDescribe()
	if version == "" {
		version = "dev"
	}
	buildTime = time.Now().UTC().Format("2006-01-02_15:04:05")
	goVersion = runtime.Version()
	return
}

func readGitDescribe() string {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func modeOrDefault(mode string) string {
	if mode == "" {
		return "default"
	}
	switch mode {
	case "dev", "release":
		return mode
	default:
		return "custom"
	}
}
