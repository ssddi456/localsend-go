package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	bubbletea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/meowrain/localsend-go/internal/config"
	"github.com/meowrain/localsend-go/internal/discovery"
	"github.com/meowrain/localsend-go/internal/handlers"
	"github.com/meowrain/localsend-go/internal/pkg/security"
	"github.com/meowrain/localsend-go/internal/pkg/server"
	"github.com/meowrain/localsend-go/internal/service"
	"github.com/meowrain/localsend-go/internal/utils/logger"
	"github.com/meowrain/localsend-go/static"
	qrcode "github.com/skip2/go-qrcode"
)

type textInputModel struct {
	value       string
	cursor      int
	placeholder string
	done        bool
}

func initialTextInputModel() textInputModel {
	return textInputModel{
		value:       "",
		cursor:      0,
		placeholder: "Enter file path...",
		done:        false,
	}
}

func (m textInputModel) Init() bubbletea.Cmd {
	return nil
}

func getPathSuggestions(input string) []string {
	if input == "" {
		input = "."
	}

	dir := input
	if !strings.HasSuffix(input, string(os.PathSeparator)) {
		dir = filepath.Dir(input)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		return nil
	}

	prefix := filepath.Clean(input)
	var suggestions []string
	for _, file := range files {
		if strings.HasPrefix(filepath.Clean(file), prefix) {
			suggestions = append(suggestions, file)
		}
	}
	return suggestions
}

func (m textInputModel) Update(msg bubbletea.Msg) (textInputModel, bubbletea.Cmd) {
	switch msg := msg.(type) {
	case bubbletea.MouseMsg:
		// 忽略鼠标事件
		return m, nil

	case bubbletea.KeyMsg:
		switch msg.String() {
		case "backspace":
			if m.cursor > 0 {
				m.value = m.value[:m.cursor-1] + m.value[m.cursor:]
				m.cursor--
			}
		case "left":
			if m.cursor > 0 {
				m.cursor--
			}
		case "right":
			if m.cursor < len(m.value) {
				m.cursor++
			}
		case "tab":
			suggestions := getPathSuggestions(m.value)
			if len(suggestions) > 0 {
				m.value = suggestions[0]
				m.cursor = len(m.value)
			}
		case "home":
			m.cursor = 0
		case "end":
			m.cursor = len(m.value)
		case "up", "down":
			// Ignore up and down key+s

		case "enter":
			m.done = true

		default:
			if msg.String() != "enter" && msg.String() != "home" && msg.String() != "end" {
				// 只允许输入有效的路径字符
				char := msg.String()
				// 检查是否是有效的路径字符
				if char == "." || char == "/" || char == "\\" || char == ":" || char == "-" || char == "_" ||
					(char >= "a" && char <= "z") || (char >= "A" && char <= "Z") || (char >= "0" && char <= "9") {
					m.value = m.value[:m.cursor] + char + m.value[m.cursor:]
					m.cursor++
				}
			}
		}
	}
	return m, nil
}

func (m textInputModel) View() string {
	if len(m.value) == 0 {
		return m.placeholder
	}
	value := m.value
	cursor := m.cursor
	if cursor > len(value) {
		cursor = len(value)
	}
	return value[:cursor] + "_" + value[cursor:]
}

func (m textInputModel) Value() string {
	return m.value
}

type model struct {
	mode        string
	choices     []string
	cursor      int
	filePrompt  bool
	textInput   textInputModel
	suggestions []string
}

func initialModel() model {
	return model{
		mode:      "",
		choices:   []string{"📤 Send", "📥 Receive", "🌎 Web", "❌ Exit"},
		cursor:    0,
		textInput: initialTextInputModel(),
	}
}

func (m model) Init() bubbletea.Cmd {
	return m.textInput.Init()
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7571F9")).
			Border(lipgloss.RoundedBorder()).
			Padding(0, 2).
			MarginBottom(1)

	menuStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			PaddingLeft(4)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7571F9")).
				PaddingLeft(2).
				SetString("❯ ")

	unselectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FAFAFA")).
				PaddingLeft(4)

	inputPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7571F9")).
				PaddingLeft(2)

	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			PaddingLeft(1)
)

func (m model) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	switch msg := msg.(type) {
	case bubbletea.MouseMsg:
		if msg.Type == bubbletea.MouseLeft {
			if msg.Y > 3 && msg.Y <= len(m.choices)+3 {
				m.cursor = msg.Y - 4
				m.mode = m.choices[m.cursor]
				if m.mode == "📤 Send" {
					m.filePrompt = true
					return m, nil
				} else {
					return m, bubbletea.Quit
				}
			}
		}

	case bubbletea.KeyMsg:
		if m.filePrompt {
			if msg.String() == "ctrl+c" {
				return m, bubbletea.Quit
			}
			m.textInput, _ = m.textInput.Update(msg)
			if m.textInput.done {
				m.mode = "📤 Send"
				return m, bubbletea.Quit
			}
			m.suggestions = getPathSuggestions(m.textInput.value)
			switch msg.String() {
			case "tab":
				if len(m.suggestions) > 0 {
					if m.cursor >= len(m.suggestions)-1 {
						m.cursor = 0
					} else {
						m.cursor++
					}
					m.textInput.value = m.suggestions[m.cursor]
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "g":
			m.cursor = 0
		case "G":
			m.cursor = len(m.choices) - 1
		case "enter":
			if m.filePrompt {
				m.textInput, _ = m.textInput.Update(msg)
				if m.textInput.done {
					m.mode = "📤 Send"
					return m, bubbletea.Quit
				}
				return m, nil
			} else {
				m.mode = m.choices[m.cursor]
				if m.mode == "📤 Send" {
					m.filePrompt = true
					return m, nil
				} else {
					return m, bubbletea.Quit
				}
			}
		case "backspace", "tab":
			if m.filePrompt {
				m.textInput, _ = m.textInput.Update(msg)
				return m, nil
			}
		case "esc":
			if m.filePrompt {
				m.filePrompt = false
				m.textInput = initialTextInputModel()
			}
		default:
			if m.filePrompt {
				m.textInput, _ = m.textInput.Update(msg)
				return m, nil
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	var s strings.Builder

	// 标题
	s.WriteString(titleStyle.Render("💫 LocalSend CLI 💫"))
	s.WriteString("\n\n")

	// 菜单
	if m.mode == "" {
		for i, choice := range m.choices {
			if i == m.cursor {
				s.WriteString(selectedItemStyle.Render(choice))
			} else {
				s.WriteString(unselectedItemStyle.Render(choice))
			}
			s.WriteString("\n")
		}
	} else {
		// 显示当前模式
		s.WriteString(menuStyle.Render(m.mode))
		s.WriteString("\n\n")

		// 文件路径输入
		if m.filePrompt {
			s.WriteString(inputPromptStyle.Render("Enter file path: "))
			s.WriteString(inputStyle.Render(m.textInput.View()))
		}
	}

	return s.String()
}

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

func SendMode(filePath string) {
	err := handlers.SendFile(filePath)
	if err != nil {
		logger.Errorf("Send failed: %v", err)
	}
}

func ExitMode() {
	fmt.Println("Exiting program...")
	os.Exit(0)
}

func flagParse(httpServer *http.ServeMux, port int, flagOpen *bool) {
	showHelp := func() {
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
	flag.Usage = showHelp
	// 解析标准flag参数
	flag.Parse()

	// 检查是否有 --help 参数
	for _, arg := range os.Args {
		if arg == "--help" || arg == "-h" {
			showHelp()
			ExitMode()
		}
	}

	if len(os.Args) > 1 {
		*flagOpen = true
		mode := os.Args[1]

		switch mode {
		case "web":
			WebServerMode(httpServer, port)
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
		case "service":
			if len(os.Args) > 2 {
				handleServiceCommand(os.Args[2])
			} else {
				logger.Error("Service command required: install, uninstall, start, stop, status")
				ExitMode()
			}
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

var port int
var daemonMode bool
var configPath string

func init() {
	flag.IntVar(&port, "port", 0, "Port to listen on (default from config or 53317)")
	flag.BoolVar(&daemonMode, "daemon", false, "Start in daemon mode (background)")
	flag.StringVar(&configPath, "config", "", "Path to custom config file (default: ./internal/config/config.yaml)")
}

func main() {
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
		port = config.ConfigData.Server.Port
		if port == 0 {
			port = 53317
		}
	}

	// Start HTTP/HTTPS server
	httpServer := server.New()

	/* Send and receive section */
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

	go func() {
		protocol := "HTTP"
		if config.ConfigData.Server.HTTPS {
			protocol = "HTTPS"
		}
		logger.Infof("Server started at :%d (%s)", port, protocol)

		// 获取本地 IP 并输出实际侦听地址
		ips, err := discovery.GetLocalIP()
		if err == nil {
			for _, ip := range ips {
				ipStr := ip.String()
				if strings.HasPrefix(ipStr, "10.") || strings.HasPrefix(ipStr, "192.168.") || strings.HasPrefix(ipStr, "172.") {
					logger.Infof("Listening on %s://%s:%d", strings.ToLower(protocol), ip, port)
				}
			}
		}

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
	} else if daemonMode {
		// 如果指定了 --daemon 标志，使用后台模式
		ReceiveModeBackground()
	}
}
