package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/meowrain/localsend-go/internal/discovery"
	"github.com/meowrain/localsend-go/internal/models"
	"github.com/meowrain/localsend-go/internal/pkg/webhook"
	"github.com/meowrain/localsend-go/internal/utils/clipboard"
	"github.com/meowrain/localsend-go/internal/utils/logger"
	"github.com/schollz/progressbar/v3"
)

var (
	v1SessionIDCounter = 0
	v1SessionMutex     sync.Mutex
	v1FileNames        = make(map[string]string) // fileID -> fileName
	v1FileTokens       = make(map[string]string) // fileID -> token
	v1SessionIP        = make(map[string]string) // track session IP for cancel verification
)

// RegisterV1Handler handles POST /api/localsend/v1/register
func RegisterV1Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req models.InfoV1
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		logger.Errorf("Invalid request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	logger.Infof("V1 Register request from: %s (%s)", req.Alias, req.DeviceModel)

	// Respond with our own info (v1 format)
	resp := models.InfoV1{
		Alias:       discovery.Message.Alias,
		DeviceModel: discovery.Message.DeviceModel,
		DeviceType:  discovery.Message.DeviceType,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// SendRequestV1Handler handles POST /api/localsend/v1/send-request
func SendRequestV1Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Info  models.InfoV1              `json:"info"`
		Files map[string]models.FileInfo `json:"files"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		logger.Errorf("Invalid request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	logger.Infof("V1 Send request from %s, device is %s", req.Info.Alias, req.Info.DeviceModel)

	// Get client IP for cancel verification
	clientIP := r.RemoteAddr
	if idx := strings.LastIndex(clientIP, ":"); idx != -1 {
		clientIP = clientIP[:idx]
	}

	v1SessionMutex.Lock()
	v1SessionIDCounter++
	sessionID := fmt.Sprintf("v1-session-%d", v1SessionIDCounter)
	v1SessionIP[clientIP] = sessionID
	v1SessionMutex.Unlock()

	// Generate tokens for each file
	tokens := make(map[string]string)
	for fileID, fileInfo := range req.Files {
		token := fmt.Sprintf("token-%s-%d", fileID, time.Now().UnixNano())
		tokens[fileID] = token

		// Store file metadata
		v1FileNames[fileID] = fileInfo.FileName
		v1FileTokens[fileID] = token

		// Handle preview data for text files
		if fileInfo.Preview != "" && strings.HasSuffix(fileInfo.FileName, ".txt") {
			logger.Success("TXT file content preview:", fileInfo.Preview)
			clipboard.WriteToClipBoard(fileInfo.Preview)
		}
	}

	// V1 response format: just a map of fileID -> token
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tokens)
}

// SendV1Handler handles POST /api/localsend/v1/send
func SendV1Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	fileID := r.URL.Query().Get("fileId")
	token := r.URL.Query().Get("token")

	if fileID == "" || token == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	// Verify token
	expectedToken, ok := v1FileTokens[fileID]
	if !ok || expectedToken != token {
		http.Error(w, "Invalid token", http.StatusForbidden)
		return
	}

	// Get file name
	fileName, ok := v1FileNames[fileID]
	if !ok {
		http.Error(w, "Invalid file ID", http.StatusBadRequest)
		return
	}

	// Create uploads directory if it doesn't exist
	err := os.MkdirAll("uploads", 0755)
	if err != nil {
		logger.Errorf("Failed to create uploads directory: %v", err)
		http.Error(w, "Failed to create directory", http.StatusInternalServerError)
		return
	}

	// Generate file path
	filePath := filepath.Join("uploads", fileName)
	dir := filepath.Dir(filePath)
	err = os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		logger.Errorf("Error creating directory: %v", err)
		http.Error(w, "Failed to create directory", http.StatusInternalServerError)
		return
	}

	// Create file
	file, err := os.Create(filePath)
	if err != nil {
		logger.Errorf("Error creating file: %v", err)
		http.Error(w, "Failed to create file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Get content length for progress bar
	contentLength := r.ContentLength
	if contentLength <= 0 {
		contentLength = 0
	}

	// Create progress bar
	bar := progressbar.NewOptions64(
		contentLength,
		progressbar.OptionSetDescription(fmt.Sprintf("下载 %s", fileName)),
		progressbar.OptionSetWidth(15),
		progressbar.OptionShowBytes(true),
		progressbar.OptionThrottle(time.Second),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerHead:    "█",
			SaucerPadding: "░",
			BarStart:      "|",
			BarEnd:        "|",
		}),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stderr, "\n")
		}),
	)

	// Read and write file with progress
	buffer := make([]byte, 2*1024*1024) // 2MB buffer
	ctx := r.Context()

	done := make(chan error, 1)

	go func() {
		for {
			select {
			case <-ctx.Done():
				done <- fmt.Errorf("transfer cancelled")
				return
			default:
				n, err := r.Body.Read(buffer)
				if err != nil && err != io.EOF {
					done <- fmt.Errorf("read file failed: %w", err)
					return
				}
				if n == 0 {
					done <- nil
					return
				}

				_, err = file.Write(buffer[:n])
				if err != nil {
					done <- fmt.Errorf("write file failed: %w", err)
					return
				}

				bar.Add(n)
			}
		}
	}()

	// Wait for completion
	err = <-done
	if err != nil {
		logger.Errorf("Transfer error: %v", err)
		os.Remove(filePath)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 获取文件大小
	fileInfo, err := os.Stat(filePath)
	var fileSize int64
	if err == nil {
		fileSize = fileInfo.Size()
	}

	logger.Success("V1 File saved to:", filePath)
	// 转换为绝对路径
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		logger.Errorf("Failed to get absolute path: %v", err)
		absPath = filePath // 如果失败，使用相对路径
	}
	// 发送成功的webhook通知
	webhook.SendUploadCompleteWebhook(absPath, fileName, fileSize, true, "")
	w.WriteHeader(http.StatusOK)
}

// CancelV1Handler handles POST /api/localsend/v1/cancel
func CancelV1Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// V1 protocol uses IP address to verify the cancel request
	clientIP := r.RemoteAddr
	if idx := strings.LastIndex(clientIP, ":"); idx != -1 {
		clientIP = clientIP[:idx]
	}

	logger.Infof("V1 Cancel request from: %s", clientIP)

	v1SessionMutex.Lock()
	sessionID, exists := v1SessionIP[clientIP]
	if exists {
		delete(v1SessionIP, clientIP)
	}
	v1SessionMutex.Unlock()

	if !exists {
		logger.Warnf("No active session found for IP: %s", clientIP)
	} else {
		logger.Infof("Cancelled session: %s", sessionID)
	}

	// Always return 200 OK for v1 cancel
	w.WriteHeader(http.StatusOK)
}
