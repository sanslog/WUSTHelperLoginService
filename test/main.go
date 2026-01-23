package main

import (
	"WUSThelperLoginSystem/test/captcha"
	"WUSThelperLoginSystem/test/sever"
	logger "WUSThelperLoginSystem/test/util"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
)

// 全局配置变量
var (
	serverPort    int
	tesseractPath string
)

const MaxRetry = 5

func main() {
	//初始化日志
	err := logger.Init("./logs")
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	//初始化ocr服务
	if err := captcha.InitOCR(tesseractPath); err != nil {
		panic(err)
	}
	defer captcha.ShutdownOCR()

	http.HandleFunc("/login", loginHandler)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("服务器启动失败: %v\n", err)
	}

}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	// 只允许 POST
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"error":"only POST is allowed"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// 限制请求体大小
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	//解析请求体json字符串
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	var data map[string]string
	if err := decoder.Decode(&data); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"bzb_jsxsd":"","SERVERID":"","error":"invalid json format"}`))
		logger.Log.Error("login---json解析失败", zap.Error(err))
		return
	}

	username := strings.TrimSpace(data["username"])
	password := strings.TrimSpace(data["password"])

	if username == "" || password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"bzb_jsxsd":"","SERVERID":"","error":"username or password is empty"}`))
		return
	}

	// 登录逻辑
	ctx := r.Context()
	bzbj, serverID, loginErr := sever.MockLogin(ctx, username, password)

	if loginErr != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"bzb_jsxsd":"","SERVERID":"","error":"` + loginErr.Error() + `"}`))
		return
	}

	// 成功返回
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(
		`{"bzb_jsxsd":"` + bzbj + `","SERVERID":"` + serverID + `","error":""}`,
	))
}

// 初始化函数：解析命令行参数
func init() {
	var customTessPath string

	// 1. 定义命令行参数
	flag.IntVar(&serverPort, "p", 0, "服务器端口(must)")
	flag.StringVar(&customTessPath, "T", "", "Tesseract可执行文件路径")

	// 2. 解析参数
	flag.Parse()

	// 3. 验证端口参数
	if serverPort <= 0 || serverPort > 65535 {
		fmt.Printf("错误: 端口号必须为 1-65535 之间的数字，当前值: %d\n", serverPort)
		printUsage()
		os.Exit(1)
	}

	// 4. 处理Tesseract路径
	var err error
	tesseractPath, err = resolveTesseractPath(customTessPath)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		printUsage()
		os.Exit(1)
	}

	// 5. 验证Tesseract可执行文件
	if err := validateTesseract(tesseractPath); err != nil {
		fmt.Printf("错误: Tesseract验证失败 - %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 配置加载成功:\n")
	fmt.Printf("  端口: %d\n", serverPort)
	fmt.Printf("  Tesseract路径: %s\n", tesseractPath)
}

// resolveTesseractPath 解析Tesseract可执行文件路径
func resolveTesseractPath(userPath string) (string, error) {
	// 如果用户提供了路径，直接使用
	if userPath != "" {
		return filepath.Clean(userPath), nil
	}

	fmt.Println("未指定Tesseract路径，开始自动查找...")

	// 根据平台使用不同的查找策略
	switch runtime.GOOS {
	case "windows":
		return findTesseractWindows()
	case "linux", "darwin": // darwin是macOS
		return findTesseractUnix()
	default:
		return "", fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// findTesseractWindows 在Windows中查找Tesseract
func findTesseractWindows() (string, error) {
	// 1. 首先尝试在PATH环境变量中查找
	if path, err := exec.LookPath("tesseract.exe"); err == nil {
		fmt.Printf("  在PATH中找到: %s\n", path)
		return path, nil
	}

	// 2. 从环境变量TESSERACT_PATH查找
	if envPath := os.Getenv("TESSERACT_PATH"); envPath != "" {
		fullPath := filepath.Join(envPath, "tesseract.exe")
		if _, err := os.Stat(fullPath); err == nil {
			fmt.Printf("  在TESSERACT_PATH环境变量中找到: %s\n", fullPath)
			return fullPath, nil
		}
	}

	return "", fmt.Errorf("未找到Tesseract.exe。请执行以下操作之一:\n" +
		"  1. 使用 -T 参数指定完整路径\n" +
		"  2. 将Tesseract添加到PATH环境变量\n")
}

// findTesseractUnix 在Linux/macOS中查找Tesseract
func findTesseractUnix() (string, error) {
	// 1. 在PATH中查找
	if path, err := exec.LookPath("tesseract"); err == nil {
		fmt.Printf("  在PATH中找到: %s\n", path)
		return path, nil
	}

	// 2. 检查常见Unix安装位置
	commonPaths := []string{
		"/usr/bin/tesseract",
		"/usr/local/bin/tesseract",
		"/opt/homebrew/bin/tesseract", // macOS Homebrew
		"/bin/tesseract",
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("  在常见路径中找到: %s\n", path)
			return path, nil
		}
	}

	// 3. 从环境变量查找
	if envPath := os.Getenv("TESSERACT_PATH"); envPath != "" {
		fullPath := filepath.Join(envPath, "tesseract")
		if _, err := os.Stat(fullPath); err == nil {
			fmt.Printf("  在TESSERACT_PATH环境变量中找到: %s\n", fullPath)
			return fullPath, nil
		}
	}

	return "", fmt.Errorf("未找到Tesseract。请安装: %s", getInstallCommand())
}

// getInstallCommand 返回平台特定的安装命令
func getInstallCommand() string {
	switch runtime.GOOS {
	case "linux":
		return "sudo apt-get install tesseract-ocr (Ubuntu/Debian) 或 sudo yum install tesseract (RHEL/CentOS)"
	case "darwin":
		return "brew install tesseract"
	case "windows":
		return "从 https://github.com/UB-Mannheim/tesseract/wiki 下载安装"
	default:
		return "请参考Tesseract官方文档"
	}
}

// validateTesseract 验证Tesseract可执行文件
func validateTesseract(path string) error {
	// 1. 检查文件是否存在且可执行
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("文件不存在或无法访问: %s", path)
	}

	// 在Unix系统检查可执行权限
	if runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("文件没有可执行权限: %s", path)
	}

	// 2. 尝试获取版本信息验证程序是否有效
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	if output, err := cmd.Output(); err != nil {
		return fmt.Errorf("无法执行Tesseract: %v", err)
	} else {
		version := strings.TrimSpace(string(output))
		if strings.Contains(version, "tesseract") {
			fmt.Printf("  Tesseract版本: %s\n", version)
			return nil
		}
		return fmt.Errorf("无效的Tesseract可执行文件")
	}
}

// printUsage 打印用法信息
func printUsage() {
	fmt.Println("\n用法:")
	fmt.Println("  myapp.exe -p <端口号> [-T <Tesseract路径>]")
	fmt.Println("\n参数说明:")
	fmt.Println("  -p  服务器端口 (1-65535，必须)")
	fmt.Println("  -T  Tesseract可执行文件路径 (可选，为空则自动查找)")
	fmt.Println("\n示例:")
	fmt.Printf("  myapp.exe -p 8080\n")
	fmt.Printf("  myapp.exe -p 8080 -T C:\\Users\\AppData\\Local\\Programs\\Tesseract-OCR\\tesseract.exe\n")
	fmt.Println("\n自动查找策略:")
	fmt.Println("  1. 检查PATH环境变量")
	fmt.Println("  2. 检查平台常见安装路径")
	fmt.Println("  3. 检查TESSERACT_PATH环境变量")
}
