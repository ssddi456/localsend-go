package tray

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/getlantern/systray"
	"github.com/meowrain/localsend-go/internal/utils/logger"
)

var (
	uploadsDirPath string
	quitChan       = make(chan struct{})
)

// Initialize initializes the system tray
func Initialize() {
	if runtime.GOOS != "windows" {
		logger.Info("System tray is only supported on Windows, skipping initialization")
		return
	}

	logger.Info("Initializing system tray...")

	// Get current working directory for uploads
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		uploadsDirPath = filepath.Join(exeDir, "uploads")
	} else {
		uploadsDirPath = "uploads"
	}

	// Ensure uploads directory exists
	os.MkdirAll(uploadsDirPath, os.ModePerm)

	systray.Run(onReady, onExit)
}

// Stop stops the system tray
func Stop() {
	systray.Quit()
}

// QuitChannel returns the channel that signals when tray is quitting
func QuitChannel() <-chan struct{} {
	return quitChan
}

// onReady is called when the system tray is ready
func onReady() {
	// Set icon
	iconPath := getIconPath()
	if iconPath != "" {
		iconData, err := os.ReadFile(iconPath)
		if err == nil {
			systray.SetIcon(iconData)
		} else {
			logger.Warn("Failed to load tray icon", "path", iconPath, "error", err)
		}
	} else {
		logger.Info("Using default tray icon")
	}

	// Set tooltip
	systray.SetTooltip("LocalSend Go - File Transfer")

	// Menu items
	mOpenUploads := systray.AddMenuItem("打开文件接收目录", "Open uploads directory")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "Quit LocalSend Go")

	// Handle menu item clicks
	go func() {
		for {
			select {
			case <-mOpenUploads.ClickedCh:
				openUploadsDirectory()
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

// onExit is called when the system tray exits
func onExit() {
	logger.Info("System tray exiting...")
	close(quitChan)
}

// openUploadsDirectory opens the uploads directory in the default file manager
func openUploadsDirectory() {
	if uploadsDirPath == "" {
		logger.Error("Uploads directory path is not set")
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", uploadsDirPath)
	case "darwin":
		cmd = exec.Command("open", uploadsDirPath)
	case "linux":
		cmd = exec.Command("xdg-open", uploadsDirPath)
	default:
		logger.Errorf("Unsupported OS for opening directory: %s", runtime.GOOS)
		return
	}

	if err := cmd.Start(); err != nil {
		logger.Errorf("Failed to open uploads directory: %v", err)
	} else {
		logger.Infof("Opened uploads directory: %s", uploadsDirPath)
	}
}

// getIconPath 获取图标文件路径
func getIconPath() string {
	// 尝试多个可能的路径
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	possiblePaths := []string{
		filepath.Join(exeDir, "assets", "tray.ico"),       // 安装后的路径
		filepath.Join("assets", "tray.ico"),               // 开发时的相对路径
		filepath.Join("..", "assets", "tray.ico"),         // 从 cmd/server 运行时
		filepath.Join(exeDir, "..", "assets", "tray.ico"), // 其他情况
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}

	return ""
}

// SetUploadsDirectory sets the custom uploads directory path
func SetUploadsDirectory(path string) {
	uploadsDirPath = path
}

// GetUploadsDirectory returns the current uploads directory path
func GetUploadsDirectory() string {
	return uploadsDirPath
}
