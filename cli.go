package main

import (
	"fmt"
	"net/http"
	"os"

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

// showHelp 显示帮助信息
func showHelp() {
	fmt.Println("Usage: <command> [arguments]")
	fmt.Println("Commands:")
	fmt.Println("  web                       Start Web mode")
	fmt.Println("  send <file_path>          Start Send mode (file path required)")
	fmt.Println("  receive                   Start Receive mode")
	fmt.Println("  daemon                    Start Receive mode in background")
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
}
