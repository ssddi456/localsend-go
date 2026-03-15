package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/meowrain/localsend-go/internal/config"
	"github.com/meowrain/localsend-go/internal/discovery"
	"github.com/meowrain/localsend-go/internal/handlers"
	"github.com/meowrain/localsend-go/internal/pkg/security"
	"github.com/meowrain/localsend-go/internal/utils/logger"
	"github.com/meowrain/localsend-go/static"
	qrcode "github.com/skip2/go-qrcode"
)

// WebServerMode Web 服务器模式
func WebServerMode(httpServer *http.ServeMux, port int) {
	err := os.MkdirAll("uploads", 0o755)
	if err != nil {
		logger.Errorf("Failed to create uploads directory: %v", err)
		return
	}
	if config.ConfigData.Functions.HttpFileServer {
		httpServer.HandleFunc("/", handlers.IndexFileHandler)
		httpServer.HandleFunc("/uploads/", handlers.FileServerHandler)
		httpServer.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static.EmbeddedStaticFiles))))
		httpServer.HandleFunc("/send", handlers.NormalSendHandler) // Upload handler
	}
	ips, _ := discovery.GetLocalIP()
	localIP := ""
	protocol := "http"
	if config.ConfigData.Server.HTTPS {
		protocol = "https"
	}

	for _, ip := range ips {
		ipStr := ip.String()
		if strings.HasPrefix(ipStr, "10.") || strings.HasPrefix(ipStr, "192.168.") {
			logger.Infof("If you opened the HTTP file server, you can view your files on %s", fmt.Sprintf("%s://%v:%d", protocol, ip, port))
		}
		if strings.HasPrefix(ipStr, "192.168.") {
			localIP = ip.String()
		}
	}
	qr, err := qrcode.New(fmt.Sprintf("%s://%s:%d", protocol, localIP, port), qrcode.Highest)
	if err != nil {
		fmt.Println("生成二维码失败:", err)
		return
	}

	// 打印二维码到终端
	fmt.Println(qr.ToString(false))
	select {}
}

// ReceiveMode 接收模式
func ReceiveMode() {
	err := os.MkdirAll("uploads", 0o755)
	if err != nil {
		logger.Errorf("Failed to create uploads directory: %v", err)
		return
	}
	discovery.ListenAndStartBroadcasts(nil)
	logger.Info("Waiting to receive files...")
	select {}
}

// ReceiveModeBackground 后台接收模式
func ReceiveModeBackground() {
	err := os.MkdirAll("uploads", 0o755)
	if err != nil {
		logger.Errorf("Failed to create uploads directory: %v", err)
		return
	}
	// 在后台运行，不显示 TUI
	logger.Info("LocalSend Receive mode started in background")
	logger.Info("Waiting to receive files...")
	discovery.ListenAndStartBroadcasts(nil)
	select {}
}

// SendMode 发送模式
func SendMode(filePath string) {
	err := handlers.SendFile(filePath)
	if err != nil {
		logger.Errorf("Send failed: %v", err)
	}
}

// ExitMode 退出模式
func ExitMode() {
	fmt.Println("Exiting program...")
	os.Exit(0)
}

// startServer 启动 HTTP/HTTPS 服务器
func startServer(httpServer *http.ServeMux, port int) {
	go func() {
		protocol := "HTTP"
		if config.ConfigData.Server.HTTPS {
			protocol = "HTTPS"
		}
		logger.Infof("Server started at :%d (%s)", port, protocol)

		if config.ConfigData.Server.HTTPS {
			// Get TLS configuration from security context
			ctx := security.GetSecurityContext()
			if ctx == nil {
				logger.Failed("Security context not initialized")
				return
			}

			tlsConfig, err := ctx.GetTLSConfig()
			if err != nil {
				logger.Failedf("Failed to get TLS config: %v", err)
				return
			}

			server := &http.Server{
				Addr:      fmt.Sprintf(":%d", port),
				Handler:   httpServer,
				TLSConfig: tlsConfig,
			}

			logger.Debugf("Certificate fingerprint: %s", ctx.CertificateHash)

			if err := server.ListenAndServeTLS("", ""); err != nil {
				log.Fatalf("HTTPS server failed: %v", err)
			}
		} else {
			if err := http.ListenAndServe(fmt.Sprintf(":%d", port), httpServer); err != nil {
				log.Fatalf("HTTP server failed: %v", err)
			}
		}
	}()
}

// setupLocalsendHandlers 设置 LocalSend 处理器
func setupLocalsendHandlers(httpServer *http.ServeMux) {
	if config.ConfigData.Functions.LocalSendServer {
		// V1 endpoints
		httpServer.HandleFunc("/api/localsend/v1/info", handlers.GetInfoV1Handler)
		httpServer.HandleFunc("/api/localsend/v1/register", handlers.RegisterV1Handler)
		httpServer.HandleFunc("/api/localsend/v1/send-request", handlers.SendRequestV1Handler)
		httpServer.HandleFunc("/api/localsend/v1/send", handlers.SendV1Handler)
		httpServer.HandleFunc("/api/localsend/v1/cancel", handlers.CancelV1Handler)

		// V2 endpoints
		httpServer.HandleFunc("/api/localsend/v2/prepare-upload", handlers.PrepareReceive)
		httpServer.HandleFunc("/api/localsend/v2/upload", handlers.ReceiveHandler)
		httpServer.HandleFunc("/api/localsend/v2/info", handlers.GetInfoV2Handler)
		httpServer.HandleFunc("/api/localsend/v2/cancel", handlers.HandleCancel)
	}
}

// setupWebServerHandlers Web 服务器处理器
func setupWebServerHandlers(httpServer *http.ServeMux) {
	if config.ConfigData.Functions.HttpFileServer {
		httpServer.HandleFunc("/", handlers.IndexFileHandler)
		httpServer.HandleFunc("/uploads/", handlers.FileServerHandler)
		httpServer.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static.EmbeddedStaticFiles))))
		httpServer.HandleFunc("/send", handlers.NormalSendHandler)
	}
}
