package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	bubbletea "github.com/charmbracelet/bubbletea"
	"github.com/meowrain/localsend-go/internal/config"
	"github.com/meowrain/localsend-go/internal/pkg/security"
	"github.com/meowrain/localsend-go/internal/pkg/server"
	"github.com/meowrain/localsend-go/internal/utils/logger"
	"golang.org/x/sys/windows/svc"
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
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
		fmt.Println("\n收到中断信号，正在退出...")
		os.Exit(0)
	}()

	logger.InitLogger()

	// Initialize security context (certificate)
	if err := security.Initialize(); err != nil {
		logger.Failedf("Failed to initialize security context: %v", err)
	}

	// Use port from flag if specified, otherwise from config
	if port == 0 {
		port = config.GetPort()
	}

	// 检查是否是 service 命令，如果是则直接处理后退出，不启动服务器
	if len(os.Args) > 1 && os.Args[1] == "service" {
		if len(os.Args) > 2 {
			handleServiceCommand(os.Args[2])
		} else {
			logger.Error("Service command required: install, uninstall, start, stop, status")
			ExitMode()
		}
		return
	}

	// Start HTTP/HTTPS server
	httpServer := server.New()

	// 设置 LocalSend 处理器
	setupLocalsendHandlers(httpServer)

	// 启动服务器
	startServer(httpServer, port)
	logger.Info(fmt.Sprintf("LocalSend CLI started. Listening on port %d daemonMode %v", port, daemonMode))

	// 处理 Windows 服务模式（使用 svc.IsWindowsService() 自动检测）
	if daemonMode {
		isService, err := svc.IsWindowsService()
		if err == nil && isService {
			handleWindowsServiceMode(httpServer, port)
			return
		}
		// 如果不是 Windows 服务，使用后台模式
		ReceiveModeBackground()
		return
	}

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
