package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	bubbletea "github.com/charmbracelet/bubbletea"
	"github.com/meowrain/localsend-go/internal/config"
	"github.com/meowrain/localsend-go/internal/pkg/security"
	"github.com/meowrain/localsend-go/internal/pkg/server"
	"github.com/meowrain/localsend-go/internal/pkg/tray"
	"github.com/meowrain/localsend-go/internal/utils/logger"
)

var port int
var daemonMode bool
var configPath string

func init() {
	flag.IntVar(&port, "port", 0, "Port to listen on (default from config or 53317)")
	flag.BoolVar(&daemonMode, "daemon", false, "Start in daemon mode (background)")
	flag.StringVar(&configPath, "config", "", "Path to custom config file (default: ./internal/config/config.yaml)")
	flag.Usage = showHelp
}

func main() {
	// 切换到可执行文件所在目录
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		if err := os.Chdir(exeDir); err != nil {
			fmt.Printf("Warning: Failed to change working directory to %s: %v\n", exeDir, err)
		} else {
			fmt.Printf("Working directory set to: %s\n", exeDir)
		}
	}

	// 手动解析命令行参数以支持 command --flag 的格式
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--port" && i+1 < len(os.Args) {
			if p, err := strconv.Atoi(os.Args[i+1]); err == nil {
				port = p
			}
			i++
		} else if arg == "--daemon" {
			daemonMode = true
		} else if arg == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
			i++
		} else if strings.HasPrefix(arg, "--port=") {
			if p, err := strconv.Atoi(arg[7:]); err == nil {
				port = p
			}
		} else if strings.HasPrefix(arg, "--config=") {
			configPath = arg[9:]
		}
	}

	// 检查是否有 --help 参数
	for _, arg := range os.Args {
		if arg == "--help" || arg == "-h" {
			showHelp()
			ExitMode()
		}
	}

	logger.Info("========================")
	logger.Info("start args is " + strings.Join(os.Args, " "))
	logger.Info("parsed config path: " + configPath)
	logger.Info("parsed port: " + fmt.Sprintf("%d", port))
	logger.Info("parsed daemon mode: " + fmt.Sprintf("%v", daemonMode))
	logger.Info("========================")

	var flagOpen bool = false

	// 设置信号处理，监听 Ctrl+C (SIGINT) 和 SIGTERM
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// 在后台处理信号
	go func() {
		sig := <-signalChan
		logger.Info(fmt.Sprintf("收到信号: %v", sig))
		fmt.Println("\n\n收到中断信号 (Ctrl+C)，正在优雅关闭...")

		// 执行清理操作
		logger.Info("正在清理资源...")

		// 这里可以添加更多清理逻辑，比如：
		// - 关闭数据库连接
		// - 保存未完成的工作
		// - 通知其他 goroutine 停止

		logger.Info("程序已安全退出")
		os.Exit(0)
	}()

	logger.InitLogger()
	go tray.Initialize()
	defer tray.Stop()

	// Initialize security context (certificate)
	if err := security.Initialize(); err != nil {
		logger.Failedf("Failed to initialize security context: %v", err)
	}

	// Use port from flag if specified, otherwise from config
	if port == 0 {
		port = config.GetPort()
	}

	// Start HTTP/HTTPS server
	httpServer := server.New()

	// 设置 LocalSend 处理器
	setupLocalsendHandlers(httpServer)

	// 启动服务器
	startServer(httpServer, port)
	logger.Info(fmt.Sprintf("LocalSend CLI started. Listening on port %d daemonMode %v", port, daemonMode))

	if daemonMode {
		ReceiveModeBackground()
	} else {
		// 参数解析
		flagParse(httpServer, port, &flagOpen)

		if !flagOpen {
			// Run Bubble Tea program
			p := bubbletea.NewProgram(initialModel(), bubbletea.WithoutSignalHandler())
			m, err := p.Run()
			if err != nil {
				log.Fatal(err)
			}

			mTyped := m.(model)
			mode := mTyped.mode

			if mode == "❌ Exit" {
				ExitMode()
			}

			if mode == "📤 Send" {
				filePath := mTyped.textInput.Value()
				if filePath == "" {
					fmt.Println("Send mode requires a file path")
					os.Exit(1)
				}
				SendMode(filePath)
			}

			if mode == "📥 Receive" {
				ReceiveMode()
			}
			if mode == "🌎 Web" {
				WebServerMode(httpServer, port)
			}
		}
	}

}
