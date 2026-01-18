package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

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
	"golang.org/x/sys/windows/svc"
)

type textInputModel struct {
	value       string
	cursor      int
	placeholder string
	done        bool
}

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
		// 配置处理器
		if err := m.configureHandlers(); err != nil {
			logger.Errorf("Failed to configure handlers: %v", err)
			closeStopChan(m.stopChan)
			return
		}

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

// configureHandlers 配置服务处理器
func (m *serviceHandler) configureHandlers() error {
	if m.httpServer == nil {
		return fmt.Errorf("httpServer is nil")
	}

	handlers := map[string]http.HandlerFunc{
		"/api/localsend/v1/info":           handlers.GetInfoV1Handler,
		"/api/localsend/v1/register":       handlers.RegisterV1Handler,
		"/api/localsend/v1/send-request":   handlers.SendRequestV1Handler,
		"/api/localsend/v1/send":           handlers.SendV1Handler,
		"/api/localsend/v1/cancel":         handlers.CancelV1Handler,
		"/api/localsend/v2/prepare-upload": handlers.PrepareReceive,
		"/api/localsend/v2/upload":         handlers.ReceiveHandler,
		"/api/localsend/v2/info":           handlers.GetInfoV2Handler,
		"/api/localsend/v2/cancel":         handlers.HandleCancel,
	}

	for path, handler := range handlers {
		if handler == nil {
			logger.Warnf("Handler for %s is nil", path)
			continue
		}
		m.httpServer.HandleFunc(path, handler)
	}

	return nil
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

// handleWindowsServiceMode 处理 Windows 服务模式启动
func handleWindowsServiceMode(httpServer *http.ServeMux, port int) {
	logger.Info("Starting in Windows Service mode...")
	h := &serviceHandler{
		httpServer: httpServer,
		port:       port,
		stopChan:   make(chan struct{}),
	}

	if err := svc.Run("LocalSendService", h); err != nil {
		logger.Failedf("Service run failed: %v", err)
		os.Exit(1)
	} else {
		logger.Info("Service stopped.")
	}
}

func flagParse(httpServer *http.ServeMux, port int, flagOpen *bool) {
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
	logger.Info("========================")
	logger.Info("start args is " + strings.Join(os.Args, " "))
	logger.Info("parsed config path: " + configPath)
	logger.Info("parsed port: " + fmt.Sprintf("%d", port))
	logger.Info("parsed daemon mode: " + fmt.Sprintf("%v", daemonMode))
	logger.Info("========================")
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
