package service

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// WindowsServiceManager Windows 平台的服务管理器
type WindowsServiceManager struct{}

const (
	// 服务名称
	ServiceName    = "LocalSendReceive"
	ServiceDisplay = "LocalSend Receive Mode"
)

// decodeWindowsOutput 正确解码 Windows 命令输出
// Windows 系统返回的输出可能是 UTF-16LE、GBK 或 UTF-8 编码
// 此函数自动检测并转换为字符串
func decodeWindowsOutput(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	// 尝试检测 UTF-16LE BOM (0xFF 0xFE)
	if len(data) > 2 && data[0] == 0xFF && data[1] == 0xFE {
		// 是 UTF-16LE 编码
		utf16Bytes := make([]uint16, len(data)/2)
		for i := 0; i < len(data)-1; i += 2 {
			utf16Bytes[i/2] = uint16(data[i]) | uint16(data[i+1])<<8
		}
		return string(utf16.Decode(utf16Bytes))
	}

	// 尝试解码为 UTF-8
	if isValidUTF8(data) {
		return string(data)
	}

	// 尝试 GBK 解码（简体中文 Windows）
	decoder := simplifiedchinese.GBK.NewDecoder()
	result, _, err := transform.String(decoder, string(data))
	if err == nil {
		return result
	}

	// 最后尝试 GB18030 编码
	decoder = simplifiedchinese.GB18030.NewDecoder()
	result, _, err = transform.String(decoder, string(data))
	if err == nil {
		return result
	}

	// 如果都失败，返回原始字符串
	return string(data)
}

// isValidUTF8 检查字节序列是否为有效的 UTF-8
func isValidUTF8(data []byte) bool {
	for i := 0; i < len(data); {
		if data[i] < 0x80 {
			i++
		} else if data[i] < 0xC0 {
			// 无效的 UTF-8 起始字节
			return false
		} else if data[i] < 0xE0 {
			if i+1 >= len(data) {
				return false
			}
			i += 2
		} else if data[i] < 0xF0 {
			if i+2 >= len(data) {
				return false
			}
			i += 3
		} else if data[i] < 0xF8 {
			if i+3 >= len(data) {
				return false
			}
			i += 4
		} else {
			return false
		}
	}
	return true
}

// Install 安装 Windows 服务
func (w *WindowsServiceManager) Install() error {
	exePath, err := GetExecutablePath()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	fmt.Printf("[DEBUG] 可执行文件路径: %s\n", exePath)

	// 使用 sc.exe 创建 Windows 服务
	// binPath 需要包含完整的命令行参数
	binPath := fmt.Sprintf("\"%s receive --daemon\"", exePath)
	cmd := exec.Command("sc.exe", "create", ServiceName,
		"binPath= "+binPath,
		"DisplayName= "+fmt.Sprintf("\"%s\"", ServiceDisplay),
		"start= delayed-auto")

	// 打印创建 service 的命令
	fmt.Printf("[DEBUG] 执行命令: %s\n", strings.Join(cmd.Args, " "))

	// 设置命令需要管理员权限
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.CombinedOutput()
	outputStr := decodeWindowsOutput(output)
	fmt.Printf("[DEBUG] 命令输出: %s\n", outputStr)
	if err != nil {
		if strings.Contains(outputStr, "already exists") {
			return fmt.Errorf("服务已存在，请先卸载旧版本: %s", outputStr)
		}
		return fmt.Errorf("创建服务失败: %w, 输出: %s", err, outputStr)
	} else {
		fmt.Println("服务创建成功: " + outputStr)
	}

	// 延迟启动服务，给 Windows 时间来注册服务
	fmt.Printf("[DEBUG] 等待服务注册...\n")
	time.Sleep(1 * time.Second)

	// 启动服务
	if err := w.Start(); err != nil {
		// 尝试卸载已创建的服务
		_ = w.Uninstall()
		return fmt.Errorf("启动服务失败: %w", err)
	}

	return nil
}

// Uninstall 卸载 Windows 服务
func (w *WindowsServiceManager) Uninstall() error {
	// 先停止服务
	_ = w.Stop()

	// 使用 sc.exe 删除服务
	cmd := exec.Command("sc.exe", "delete", ServiceName)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.CombinedOutput()
	outputStr := decodeWindowsOutput(output)
	if err != nil {
		if strings.Contains(outputStr, "does not exist") {
			return fmt.Errorf("服务不存在")
		}
		return fmt.Errorf("删除服务失败: %w, 输出: %s", err, outputStr)
	}

	return nil
}

// Start 启动 Windows 服务
func (w *WindowsServiceManager) Start() error {
	fmt.Printf("[DEBUG] 正在启动服务: %s\n", ServiceName)

	cmd := exec.Command("sc.exe", "start", ServiceName)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.CombinedOutput()
	outputStr := decodeWindowsOutput(output)
	fmt.Printf("[DEBUG] 启动命令输出: %s\n", outputStr)

	if err != nil {
		if strings.Contains(outputStr, "already running") {
			return fmt.Errorf("服务已在运行")
		}
		if strings.Contains(outputStr, "1053") {
			// 1053: 服务没有及时响应启动或控制请求
			fmt.Printf("[DEBUG] 收到 1053 错误，等待服务启动...\n")
			time.Sleep(3 * time.Second)
			// 再次检查服务状态
			status, err := w.Status()
			if err == nil && strings.Contains(status, "运行中") {
				fmt.Printf("[DEBUG] 服务已成功启动\n")
				return nil
			}
			return fmt.Errorf("启动服务失败: 服务没有及时响应启动请求，输出: %s", outputStr)
		}
		return fmt.Errorf("启动服务失败: %w, 输出: %s", err, outputStr)
	}

	// 等待服务真正启动
	fmt.Printf("[DEBUG] 等待服务启动...\n")
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		status, err := w.Status()
		fmt.Printf("[DEBUG] 第 %d 次检查 - 状态: %s, 错误: %v\n", i+1, status, err)
		if err == nil && strings.Contains(status, "运行中") {
			fmt.Printf("[DEBUG] 服务已启动\n")
			return nil
		}
	}

	fmt.Printf("[DEBUG] 等待超时，但启动命令已执行\n")
	return nil
}

// Stop 停止 Windows 服务
func (w *WindowsServiceManager) Stop() error {
	cmd := exec.Command("sc.exe", "stop", ServiceName)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.CombinedOutput()
	outputStr := decodeWindowsOutput(output)
	if err != nil {
		return fmt.Errorf("停止服务失败: %w, 输出: %s", err, outputStr)
	}

	return nil
}

// Status 查询 Windows 服务状态
func (w *WindowsServiceManager) Status() (string, error) {
	cmd := exec.Command("sc.exe", "query", ServiceName)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.CombinedOutput()
	outputStr := decodeWindowsOutput(output)

	if err != nil {
		if strings.Contains(outputStr, "does not exist") {
			return "未安装", nil
		}
		return "", fmt.Errorf("查询服务失败: %w", err)
	}

	if strings.Contains(outputStr, "RUNNING") {
		return "运行中", nil
	} else if strings.Contains(outputStr, "STOPPED") {
		return "已停止", nil
	}

	return outputStr, nil
}

// CheckAdmin 检查是否有管理员权限
func CheckAdmin() bool {
	_, err := os.Open("\\\\.\\pipe\\")
	if err != nil {
		return false
	}
	return true
}
