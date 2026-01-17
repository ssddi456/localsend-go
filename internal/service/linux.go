package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LinuxServiceManager Linux 平台的服务管理器（systemd）
type LinuxServiceManager struct{}

const (
	SystemdServicePath = "/etc/systemd/system/localsend-receive.service"
	ServiceUnit        = "localsend-receive"
)

// Install 安装 systemd 服务
func (l *LinuxServiceManager) Install() error {
	exePath, err := GetExecutablePath()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	// 创建 systemd 服务文件内容
	serviceContent := fmt.Sprintf(`[Unit]
Description=LocalSend Receive Mode Service
After=network.target

[Service]
Type=simple
User=%s
ExecStart=%s receive --daemon
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, getCurrentUser(), exePath)

	// 写入服务文件（需要 root 权限）
	cmd := exec.Command("sudo", "tee", SystemdServicePath)
	cmd.Stdin = strings.NewReader(serviceContent)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("写入服务文件失败: %w", err)
	}

	// 重新加载 systemd 配置
	reloadCmd := exec.Command("sudo", "systemctl", "daemon-reload")
	if err := reloadCmd.Run(); err != nil {
		return fmt.Errorf("重新加载 systemd 配置失败: %w", err)
	}

	// 启用开机自启
	enableCmd := exec.Command("sudo", "systemctl", "enable", ServiceUnit)
	if err := enableCmd.Run(); err != nil {
		return fmt.Errorf("启用开机自启失败: %w", err)
	}

	// 启动服务
	return l.Start()
}

// Uninstall 卸载 systemd 服务
func (l *LinuxServiceManager) Uninstall() error {
	// 停止服务
	_ = l.Stop()

	// 禁用开机自启
	disableCmd := exec.Command("sudo", "systemctl", "disable", ServiceUnit)
	_ = disableCmd.Run()

	// 删除服务文件
	rmCmd := exec.Command("sudo", "rm", "-f", SystemdServicePath)
	if err := rmCmd.Run(); err != nil {
		return fmt.Errorf("删除服务文件失败: %w", err)
	}

	// 重新加载 systemd 配置
	reloadCmd := exec.Command("sudo", "systemctl", "daemon-reload")
	_ = reloadCmd.Run()

	return nil
}

// Start 启动 systemd 服务
func (l *LinuxServiceManager) Start() error {
	cmd := exec.Command("sudo", "systemctl", "start", ServiceUnit)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("启动服务失败: %w, 输出: %s", err, string(output))
	}
	return nil
}

// Stop 停止 systemd 服务
func (l *LinuxServiceManager) Stop() error {
	cmd := exec.Command("sudo", "systemctl", "stop", ServiceUnit)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("停止服务失败: %w, 输出: %s", err, string(output))
	}
	return nil
}

// Status 查询 systemd 服务状态
func (l *LinuxServiceManager) Status() (string, error) {
	cmd := exec.Command("sudo", "systemctl", "is-active", ServiceUnit)
	output, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(output))

	if err != nil {
		// 服务不存在或未运行
		if strings.Contains(outputStr, "inactive") || strings.Contains(outputStr, "unknown") {
			return "未运行或未安装", nil
		}
		return "", fmt.Errorf("查询服务失败: %w", err)
	}

	if outputStr == "active" {
		return "运行中", nil
	}

	return outputStr, nil
}

// getCurrentUser 获取当前用户
func getCurrentUser() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("LOGNAME"); user != "" {
		return user
	}
	return "root"
}

// GetServiceLogPath 获取服务日志路径
func GetServiceLogPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "/var/log/localsend-receive.log"
	}
	return filepath.Join(homeDir, ".localsend", "receive.log")
}
