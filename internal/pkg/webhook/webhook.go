package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/meowrain/localsend-go/internal/config"
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

// buildPlaceholderMap creates a map of placeholder values from the payload
func buildPlaceholderMap(payload UploadCompletePayload) map[string]string {
	return map[string]string{
		"FilePath":      payload.FilePath,
		"FileName":      payload.FileName,
		"FileSize":      toString(payload.FileSize),
		"CompletedAt":   payload.CompletedAt.String(),
		"CompletedAtMS": toString(payload.CompletedAtMS),
		"Status":        payload.Status,
		"ErrorMessage":  payload.ErrorMessage,
	}
}

// toString converts int64 to string
func toString(i int64) string {
	return fmt.Sprintf("%d", i)
}

// replacePlaceholders replaces placeholders in text with values from the map
func replacePlaceholders(text string, placeholders map[string]string) string {
	result := text
	for key, value := range placeholders {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}
	return result
}

// SendUploadCompleteWebhook sends webhook notifications to all configured endpoints
func SendUploadCompleteWebhook(filePath string, fileName string, fileSize int64, success bool, errMsg string) {
	endpoints := config.GetWebhookEndpoints()
	if len(endpoints) == 0 {
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

	// Build placeholder map
	placeholders := buildPlaceholderMap(payload)

	// Send to all configured endpoints
	for i, endpoint := range endpoints {
		if endpoint.URL == "" {
			continue
		}
		go sendWebhookToEndpoint(i, endpoint, payload, placeholders)
	}
}

// sendWebhookToEndpoint sends webhook to a single endpoint with its parameters
func sendWebhookToEndpoint(index int, endpoint config.WebhookEndpoint, payload UploadCompletePayload, placeholders map[string]string) {
	// Prepare the main webhook data
	webhookData := map[string]interface{}{
		"payload": payload,
	}

	// Add custom parameters with placeholder replacement
	if len(endpoint.Params) > 0 {
		customParams := replacePlaceholdersInParams(endpoint.Params, placeholders)
		// Convert any map[interface{}]interface{} to map[string]interface{} for JSON compatibility
		customParams = convertToStringMap(customParams).(map[string]interface{})
		// Copy customParams properties to webhookData
		maps.Copy(webhookData, customParams)
	}

	jsonData, err := json.Marshal(webhookData)
	if err != nil {
		logger.Errorf("Failed to marshal webhook payload for endpoint %d: %v", index, err)
		return
	}

	sendWebhookJSON(index, endpoint.URL, jsonData)
}

// replacePlaceholdersInParams recursively replaces placeholders in parameter values
func replacePlaceholdersInParams(params map[string]interface{}, placeholders map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range params {
		result[key] = replacePlaceholdersInValue(value, placeholders)
	}
	return result
}

// replacePlaceholdersInValue recursively replaces placeholders in any type of value
func replacePlaceholdersInValue(value interface{}, placeholders map[string]string) interface{} {
	switch v := value.(type) {
	case string:
		return replacePlaceholders(v, placeholders)
	case map[string]interface{}:
		return replacePlaceholdersInParams(v, placeholders)
	case map[interface{}]interface{}:
		// Convert map[interface{}]interface{} to map[string]interface{}
		result := make(map[string]interface{})
		for key, val := range v {
			keyStr := fmt.Sprintf("%v", key)
			result[keyStr] = replacePlaceholdersInValue(val, placeholders)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = replacePlaceholdersInValue(item, placeholders)
		}
		return result
	default:
		return value
	}
}

// convertToStringMap recursively converts map[interface{}]interface{} to map[string]interface{}
// This is necessary because yaml.Unmarshal produces map[interface{}]interface{} which JSON encoder cannot handle
func convertToStringMap(data interface{}) interface{} {
	switch v := data.(type) {
	case map[interface{}]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			keyStr := fmt.Sprintf("%v", key)
			result[keyStr] = convertToStringMap(val)
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			result[key] = convertToStringMap(val)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = convertToStringMap(item)
		}
		return result
	default:
		return v
	}
}

// sendWebhookJSON sends the webhook as JSON via POST
func sendWebhookJSON(index int, apiEndpoint string, jsonData []byte) {
	req, err := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Errorf("Failed to create webhook request for endpoint %d (%s): %v", index, apiEndpoint, err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("Failed to send webhook to endpoint %d (%s): %v", index, apiEndpoint, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		logger.Infof("Webhook sent successfully to endpoint %d: %s", index, apiEndpoint)
	} else {
		logger.Warnf("Webhook to endpoint %d (%s) returned status code: %d", index, apiEndpoint, resp.StatusCode)
	}
}
