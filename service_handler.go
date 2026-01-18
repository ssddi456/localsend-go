package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/meowrain/localsend-go/internal/config"
	"github.com/meowrain/localsend-go/internal/discovery"
	"github.com/meowrain/localsend-go/internal/pkg/security"
	"github.com/meowrain/localsend-go/internal/utils/logger"
	"golang.org/x/sys/windows/svc"
)

// serviceHandler 实现 Windows 服务处理接口
type serviceHandler struct {
	httpServer *http.ServeMux
	port       int
	stopChan   chan struct{}
}

// Execute 实现 svc.Handler 接口
func (m *serviceHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	// 发送启动中状态
	if err := sendStatus(changes, svc.Status{State: svc.StartPending}); err != nil {
		logger.Errorf("Failed to send StartPending status: %v", err)
		return true, uint32(1)
	}

	// 启动后台服务
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("Service goroutine panic: %v", r)
			}
		}()
		m.startReceiveServer()
	}()

	// 发送运行状态
	if err := sendStatus(changes, svc.Status{State: svc.Running, Accepts: cmdsAccepted}); err != nil {
		logger.Errorf("Failed to send Running status: %v", err)
		return true, uint32(1)
	}
	logger.Info("Windows Service started successfully")

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				if err := sendStatus(changes, c.CurrentStatus); err != nil {
					logger.Errorf("Failed to send Interrogate status: %v", err)
				}

			case svc.Stop, svc.Shutdown:
				logger.Info("Service stopping...")
				if err := sendStatus(changes, svc.Status{State: svc.StopPending}); err != nil {
					logger.Errorf("Failed to send StopPending status: %v", err)
				}
				closeStopChan(m.stopChan)
				if err := sendStatus(changes, svc.Status{State: svc.Stopped}); err != nil {
					logger.Errorf("Failed to send Stopped status: %v", err)
				}
				return false, 0

			default:
				logger.Warnf("Unexpected service control: %v", c)
			}

		case <-m.stopChan:
			if err := sendStatus(changes, svc.Status{State: svc.Stopped}); err != nil {
				logger.Errorf("Failed to send final Stopped status: %v", err)
			}
			return false, 0
		}
	}
}

// sendStatus 安全地发送服务状态
func sendStatus(changes chan<- svc.Status, status svc.Status) error {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("panic while sending status: %v", r)
		}
	}()

	select {
	case changes <- status:
		return nil
	case <-time.After(time.Second * 5):
		return fmt.Errorf("timeout sending status")
	}
}

// closeStopChan 安全地关闭 stopChan
func closeStopChan(ch chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			logger.Warnf("panic while closing stopChan: %v", r)
		}
	}()

	select {
	case ch <- struct{}{}:
	default:
		// 如果 channel 已关闭或已满，不做操作
	}
}

// startReceiveServer 启动接收服务器
func (m *serviceHandler) startReceiveServer() {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("startReceiveServer panic: %v", r)
		}
	}()

	// 创建上传目录
	if err := os.MkdirAll("uploads", 0o755); err != nil {
		logger.Errorf("Failed to create uploads directory: %v", err)
		closeStopChan(m.stopChan)
		return
	}

	// 启动 HTTP/HTTPS 服务器（如果已配置）
	if config.ConfigData.Functions.LocalSendServer {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("HTTP server goroutine panic: %v", r)
				}
			}()
			m.startHTTPServer()
		}()
	}

	// 启动广播发现
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("Broadcast goroutine panic: %v", r)
			}
		}()
		discovery.ListenAndStartBroadcasts(nil)
	}()

	logger.Info("Service: Waiting to receive files...")

	// 持续等待，直到收到停止信号
	<-m.stopChan
	logger.Info("Service stopped, cleaning up...")
}

// startHTTPServer 启动 HTTP/HTTPS 服务器
func (m *serviceHandler) startHTTPServer() {
	protocol := "HTTP"
	if config.ConfigData.Server.HTTPS {
		protocol = "HTTPS"
	}
	logger.Infof("Service: Server started at :%d (%s)", m.port, protocol)

	if config.ConfigData.Server.HTTPS {
		if err := m.startHTTPSServer(); err != nil {
			logger.Failedf("HTTPS server failed: %v", err)
		}
	} else {
		if err := http.ListenAndServe(fmt.Sprintf(":%d", m.port), m.httpServer); err != nil {
			logger.Failedf("HTTP server failed: %v", err)
		}
	}
}

// startHTTPSServer 启动 HTTPS 服务器
func (m *serviceHandler) startHTTPSServer() error {
	ctx := security.GetSecurityContext()
	if ctx == nil {
		return fmt.Errorf("security context not initialized")
	}

	tlsConfig, err := ctx.GetTLSConfig()
	if err != nil {
		return fmt.Errorf("failed to get TLS config: %w", err)
	}

	server := &http.Server{
		Addr:      fmt.Sprintf(":%d", m.port),
		Handler:   m.httpServer,
		TLSConfig: tlsConfig,
	}

	logger.Debugf("Certificate fingerprint: %s", ctx.CertificateHash)

	if err := server.ListenAndServeTLS("", ""); err != nil {
		return fmt.Errorf("TLS server error: %w", err)
	}

	return nil
}
