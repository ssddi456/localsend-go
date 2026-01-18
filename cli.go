package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/meowrain/localsend-go/internal/service"
	"github.com/meowrain/localsend-go/internal/utils/logger"
)

// flagParse 参数解析
func flagParse(httpServer *http.ServeMux, port int, flagOpen *bool) {
	if len(os.Args) > 1 {
		*flagOpen = true
		mode := os.Args[1]

		switch mode {
		case "web":
			// Web 模式参数处理
			if httpServer != nil {
				WebServerMode(httpServer, port)
			}
		case "send":
			filePath := ""
			if len(os.Args) > 2 {
				filePath = os.Args[2]
				SendMode(filePath)
			} else {
				logger.Error("Need file path")
				ExitMode()
			}
		case "receive":
			ReceiveMode()
		case "daemon":
			ReceiveModeBackground()
		case "help":
			showHelp()
			ExitMode()
		}
	}
}

// handleServiceCommand 处理服务管理命令
func handleServiceCommand(command string) {
	switch command {
	case "install":
		fmt.Println("Installing LocalSend Receive mode as system service...")
		if err := service.InstallService(); err != nil {
			fmt.Printf("❌ Install failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Service installed successfully!")
		fmt.Println("The service will start automatically on boot in Receive mode.")

	case "uninstall":
		fmt.Println("Uninstalling LocalSend Receive mode service...")
		if err := service.UninstallService(); err != nil {
			fmt.Printf("❌ Uninstall failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Service uninstalled successfully!")

	case "start":
		fmt.Println("Starting LocalSend Receive mode service...")
		if err := service.StartInBackground(); err != nil {
			fmt.Printf("❌ Start failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Service started successfully!")

	case "stop":
		fmt.Println("Stopping LocalSend Receive mode service...")
		if err := service.StopService(); err != nil {
			fmt.Printf("❌ Stop failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Service stopped successfully!")

	case "status":
		status, err := service.GetServiceStatus()
		if err != nil {
			fmt.Printf("❌ Status check failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Service Status: %s\n", status)

	default:
		fmt.Printf("Unknown service command: %s\n", command)
		fmt.Println("Available commands: install, uninstall, start, stop, status")
		os.Exit(1)
	}

	ExitMode()
}

// showHelp 显示帮助信息
func showHelp() {
	fmt.Println("Usage: <command> [arguments]")
	fmt.Println("Commands:")
	fmt.Println("  web                       Start Web mode")
	fmt.Println("  send <file_path>          Start Send mode (file path required)")
	fmt.Println("  receive                   Start Receive mode")
	fmt.Println("  daemon                    Start Receive mode in background (for service)")
	fmt.Println("  service install           Install Receive mode as system service (auto-start on boot)")
	fmt.Println("  service uninstall         Uninstall system service")
	fmt.Println("  service start             Start the system service")
	fmt.Println("  service stop              Stop the system service")
	fmt.Println("  service status            Check system service status")
	fmt.Println("  help                      Display this help information")
	fmt.Println("Options:")
	fmt.Println("  --help                    Display this help information")
	fmt.Println("  --port=<number>           Specify server port (default: 53317)")
	fmt.Println("  --config=<path>           Specify custom config file path")
	fmt.Println("  --daemon                  Start Receive mode in background")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  localsend-go receive")
	fmt.Println("  localsend-go send /path/to/file")
	fmt.Println("  localsend-go --config=/etc/localsend/config.yaml receive")
	fmt.Println("  localsend-go service install   (requires admin/root)")
	fmt.Println("  localsend-go service status")
}
