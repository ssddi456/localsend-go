package config

import (
	"embed"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/meowrain/localsend-go/internal/pkg/security"
	"github.com/meowrain/localsend-go/internal/utils/logger"
	"gopkg.in/yaml.v2"
)

//go:embed config.yaml
var embeddedConfig embed.FS

const (
	ServerPort = 53317
)

type Config struct {
	DeviceName   string `yaml:"device_name"`
	NameOfDevice string // Actual device name used in runtime
	Server       struct {
		HTTPS bool `yaml:"https"`
		Port  int  `yaml:"port"`
	} `yaml:"server"`
	Functions struct {
		HttpFileServer  bool `yaml:"http_file_server"`
		LocalSendServer bool `yaml:"local_send_server"`
	} `yaml:"functions"`
	Webhook struct {
		UploadCompleteURL string `yaml:"upload_complete_url"`
	} `yaml:"webhook"`
}

// random device name
var (
	adjectives = []string{
		"Happy", "Swift", "Silent", "Clever", "Brave",
		"Gentle", "Wise", "Calm", "Lucky", "Proud",
	}
	nouns = []string{
		"Phoenix", "Wolf", "Eagle", "Lion", "Owl",
		"Shark", "Tiger", "Bear", "Hawk", "Fox",
	}
)

var ConfigData Config

// Configuration file watcher state
var (
	configMutex    sync.RWMutex
	configFilePath string
	watchStopChan  chan struct{}
	isWatching     bool
	watchers       []ConfigChangeListener
)

// ConfigChangeListener is called when config changes
type ConfigChangeListener func(oldConfig, newConfig Config)

// random device name generator
func generateRandomName() string {
	localRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	adj := adjectives[localRand.Intn(len(adjectives))]
	noun := nouns[localRand.Intn(len(nouns))]
	return fmt.Sprintf("%s %s", adj, noun)
}

func init() {
	configFilePath = "internal/config/config.yaml"

	// 在初始化日志之前设置自定义配置文件路径（如果指定的话）
	// 这需要在 logger.InitLogger() 之前完成，因为配置初始化发生在 init() 中
	// 注意：flag.Parse() 会在 flagParse() 中调用，所以需要手动解析 config 标志
	for i, arg := range os.Args {
		if strings.HasPrefix(arg, "--config=") {
			configFilePath = arg[9:] // 去掉 "--config=" 前缀
			break
		} else if arg == "--config" && i+1 < len(os.Args) {
			configFilePath = os.Args[i+1]
			break
		}
	}

	// 加载初始配置
	if err := loadConfigFromFile(); err != nil {
		logger.Failedf("Failed to load config: %v", err)
	}

	// Use configured device name if provided, otherwise generate a random one
	if ConfigData.DeviceName != "" {
		ConfigData.NameOfDevice = ConfigData.DeviceName
		logger.Debug("Using configured device name: " + ConfigData.NameOfDevice)
	} else {
		ConfigData.NameOfDevice = generateRandomName()
		logger.Debug("Using randomly generated device name: " + ConfigData.NameOfDevice)
	}
}

// loadConfigFromFile reads and parses configuration from file
func loadConfigFromFile() error {
	var bytes []byte
	var err error

	// 尝试从文件系统读取本地配置文件
	bytes, err = os.ReadFile(configFilePath)
	if err != nil {
		logger.Debug("Read config.yaml from file system failed, using embedded config. Error: " + err.Error())
		bytes, err = embeddedConfig.ReadFile("config.yaml")
		if err != nil {
			return fmt.Errorf("cannot read embedded config file: %v", err)
		}
	}

	var newConfig Config
	if err := yaml.Unmarshal(bytes, &newConfig); err != nil {
		return fmt.Errorf("failed to parse config file: %v", err)
	}

	// Set defaults if not specified
	if newConfig.Server.Port == 0 {
		newConfig.Server.Port = 53317
	}

	configMutex.Lock()
	oldConfig := ConfigData
	ConfigData = newConfig
	configMutex.Unlock()

	logger.Debug("Config loaded successfully")

	// Notify listeners if config has changed
	if !configEqual(oldConfig, newConfig) && isWatching {
		notifyConfigChange(oldConfig, newConfig)
	}

	return nil
}

// configEqual compares two Config structs
func configEqual(a, b Config) bool {
	return a.DeviceName == b.DeviceName &&
		a.NameOfDevice == b.NameOfDevice &&
		a.Server.HTTPS == b.Server.HTTPS &&
		a.Server.Port == b.Server.Port &&
		a.Functions.HttpFileServer == b.Functions.HttpFileServer &&
		a.Functions.LocalSendServer == b.Functions.LocalSendServer &&
		a.Webhook.UploadCompleteURL == b.Webhook.UploadCompleteURL
}

// notifyConfigChange calls all registered listeners
func notifyConfigChange(oldConfig, newConfig Config) {
	configMutex.RLock()
	listenersCopy := make([]ConfigChangeListener, len(watchers))
	copy(listenersCopy, watchers)
	configMutex.RUnlock()

	for _, listener := range listenersCopy {
		go listener(oldConfig, newConfig)
	}
}

func GetCertificateFingerprint() string {
	ctx := security.GetSecurityContext()
	if ctx != nil {
		return ctx.CertificateHash
	}
	return "no-certificate"
}

func GetPort() int {
	if ConfigData.Server.Port != 0 {
		return ConfigData.Server.Port
	}
	return ServerPort
}

func GetWebhookURL() string {
	return ConfigData.Webhook.UploadCompleteURL
}

func GetProtocol() string {
	if ConfigData.Server.HTTPS {
		return "https"
	}
	return "http"
}

// StartWatching starts monitoring the configuration file for changes
func StartWatching() {
	configMutex.Lock()
	if isWatching {
		configMutex.Unlock()
		logger.Debug("Config watcher is already running")
		return
	}
	isWatching = true
	watchStopChan = make(chan struct{})
	configMutex.Unlock()

	logger.Debug("Starting config file watcher")

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		var lastModTime time.Time
		if info, err := os.Stat(configFilePath); err == nil {
			lastModTime = info.ModTime()
		}

		for {
			select {
			case <-watchStopChan:
				logger.Debug("Config file watcher stopped")
				return
			case <-ticker.C:
				info, err := os.Stat(configFilePath)
				if err != nil {
					// File might be temporarily inaccessible, continue watching
					continue
				}

				currentModTime := info.ModTime()
				if currentModTime.After(lastModTime) {
					lastModTime = currentModTime
					logger.Debug("Config file changed, reloading...")

					// Give the file system time to flush
					time.Sleep(100 * time.Millisecond)

					if err := loadConfigFromFile(); err != nil {
						logger.Failedf("Failed to reload config: %v", err)
					} else {
						logger.Debug("Config reloaded successfully")
					}
				}
			}
		}
	}()
}

// StopWatching stops monitoring the configuration file
func StopWatching() {
	configMutex.Lock()
	if !isWatching {
		configMutex.Unlock()
		return
	}
	isWatching = false
	if watchStopChan != nil {
		close(watchStopChan)
		watchStopChan = nil
	}
	configMutex.Unlock()
}

// RegisterChangeListener registers a function to be called when config changes
func RegisterChangeListener(listener ConfigChangeListener) {
	configMutex.Lock()
	defer configMutex.Unlock()
	watchers = append(watchers, listener)
}

// RemoveChangeListener removes a previously registered listener
func RemoveChangeListener(index int) {
	configMutex.Lock()
	defer configMutex.Unlock()
	if index >= 0 && index < len(watchers) {
		watchers = append(watchers[:index], watchers[index+1:]...)
	}
}

// GetConfig returns a copy of the current configuration
func GetConfig() Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return ConfigData
}
