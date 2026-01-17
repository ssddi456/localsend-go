package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/meowrain/localsend-go/internal/utils/logger"
)

// UploadCompletePayload represents the webhook payload sent when file upload completes
type UploadCompletePayload struct {
	FilePath      string    `json:"file_path"`
	FileName      string    `json:"file_name"`
	FileSize      int64     `json:"file_size"`
	CompletedAt   time.Time `json:"completed_at"`
	CompletedAtMS int64     `json:"completed_at_ms"`
	Status        string    `json:"status"` // "success" or "failed"
	ErrorMessage  string    `json:"error_message,omitempty"`
}

// SendUploadCompleteWebhook sends a webhook notification when file upload completes
func SendUploadCompleteWebhook(webhookURL string, filePath string, fileName string, fileSize int64, success bool, errMsg string) {
	if webhookURL == "" {
		return
	}

	status := "success"
	if !success {
		status = "failed"
	}

	now := time.Now()
	payload := UploadCompletePayload{
		FilePath:      filePath,
		FileName:      fileName,
		FileSize:      fileSize,
		CompletedAt:   now,
		CompletedAtMS: now.UnixMilli(),
		Status:        status,
		ErrorMessage:  errMsg,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf("Failed to marshal webhook payload: %v", err)
		return
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Errorf("Failed to create webhook request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("Failed to send webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		logger.Infof("Webhook sent successfully: %s", webhookURL)
	} else {
		logger.Warnf("Webhook returned status code: %d", resp.StatusCode)
	}
}
