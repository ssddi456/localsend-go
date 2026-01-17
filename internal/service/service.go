package service

import (
	"os"
	"path/filepath"
	"runtime"
)

// ServiceManager 管理系统服务的接口
type ServiceManager interface {
	Install() error
	Uninstall() error
	Start() error
	Stop() error
	Status() (string, error)
}

// GetServiceManager 根据操作系统返回相应的服务管理器
func GetServiceManager() ServiceManager {
	if runtime.GOOS == "windows" {
		return &WindowsServiceManager{}
	}
	return &LinuxServiceManager{}
}

// GetExecutablePath 获取当前可执行文件的完整路径
func GetExecutablePath() (string, error) {
	ex, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(ex)
}

// StartInBackground 在后台启动 receive 模式
func StartInBackground() error {
	sm := GetServiceManager()
	return sm.Start()
}

// StopService 停止 receive 模式
func StopService() error {
	sm := GetServiceManager()
	return sm.Stop()
}

// GetServiceStatus 获取服务状态
func GetServiceStatus() (string, error) {
	sm := GetServiceManager()
	return sm.Status()
}

// InstallService 安装服务
func InstallService() error {
	sm := GetServiceManager()
	return sm.Install()
}

// UninstallService 卸载服务
func UninstallService() error {
	sm := GetServiceManager()
	return sm.Uninstall()
}
