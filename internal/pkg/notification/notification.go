package notification

import (
	"fmt"
	"runtime"

	"github.com/go-toast/toast"
	"github.com/meowrain/localsend-go/internal/utils/logger"
)

// NotificationConfig holds configuration for notifications
type NotificationConfig struct {
	Title    string
	Message  string
	AppID    string
	IconPath string // Optional path to icon file
	Duration string // "short" or "long"
}

// SendNotification sends a desktop notification
func SendNotification(config NotificationConfig) error {
	if runtime.GOOS != "windows" {
		logger.Info("Desktop notifications are only supported on Windows, skipping")
		return nil
	}

	notification := toast.Notification{
		AppID:   getAppID(config.AppID),
		Title:   config.Title,
		Message: config.Message,
		Icon:    getIconPath(config.IconPath),
		Audio:   toast.Default,
	}

	err := notification.Push()
	if err != nil {
		logger.Errorf("Failed to send notification: %v", err)
		return err
	}

	logger.Infof("Notification sent: %s", config.Title)
	return nil
}

// SendFileReceivedNotification sends a notification when a file is received
func SendFileReceivedNotification(fileName string, filePath string) error {
	config := NotificationConfig{
		Title:    "文件接收完成",
		Message:  fmt.Sprintf("文件 '%s' 已保存到 uploads 目录", fileName),
		AppID:    "LocalSend Go",
		Duration: "short",
	}
	return SendNotification(config)
}

// SendErrorNotification sends a notification when an error occurs
func SendErrorNotification(message string) error {
	config := NotificationConfig{
		Title:    "LocalSend 错误",
		Message:  message,
		AppID:    "LocalSend Go",
		Duration: "short",
	}
	return SendNotification(config)
}

// getAppID returns the AppID for the notification
// If not provided, use a default value
func getAppID(appID string) string {
	if appID == "" {
		return "LocalSend Go"
	}
	return appID
}

// getIconPath returns the icon path for the notification
// If not provided, return empty string (will use default icon)
func getIconPath(iconPath string) string {
	return iconPath
}
