package service

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// WindowsServiceManager Windows 平台的服务管理器
type WindowsServiceManager struct{}

const (
	// 服务名称
	ServiceName    = "LocalSendReceive"
	ServiceDisplay = "LocalSend Receive Mode"
)

// Install 安装 Windows 服务
func (w *WindowsServiceManager) Install() error {
	exePath, err := GetExecutablePath()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	// 使用 sc.exe 创建 Windows 服务
	cmd := exec.Command("sc.exe", "create", ServiceName,
		"binPath="+fmt.Sprintf("\"%s receive --daemon\"", exePath),
		"DisplayName="+ServiceDisplay,
		"start=auto")

	// 设置命令需要管理员权限
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "already exists") {
			return fmt.Errorf("服务已存在，请先卸载旧版本: %s", string(output))
		}
		return fmt.Errorf("创建服务失败: %w, 输出: %s", err, string(output))
	}

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
	if err != nil {
		if strings.Contains(string(output), "does not exist") {
			return fmt.Errorf("服务不存在")
		}
		return fmt.Errorf("删除服务失败: %w, 输出: %s", err, string(output))
	}

	return nil
}

// Start 启动 Windows 服务
func (w *WindowsServiceManager) Start() error {
	cmd := exec.Command("sc.exe", "start", ServiceName)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "already running") {
			return fmt.Errorf("服务已在运行")
		}
		return fmt.Errorf("启动服务失败: %w, 输出: %s", err, string(output))
	}

	return nil
}

// Stop 停止 Windows 服务
func (w *WindowsServiceManager) Stop() error {
	cmd := exec.Command("sc.exe", "stop", ServiceName)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("停止服务失败: %w, 输出: %s", err, string(output))
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
	outputStr := string(output)

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
