package config

import (
	"embed"
	"fmt"
	"math/rand"
	"os"
	"strings"
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

// random device name generator
func generateRandomName() string {
	localRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	adj := adjectives[localRand.Intn(len(adjectives))]
	noun := nouns[localRand.Intn(len(nouns))]
	return fmt.Sprintf("%s %s", adj, noun)
}

func init() {
	var bytes []byte
	var err error

	var configPath string = "internal/config/config.yaml"

	// 在初始化日志之前设置自定义配置文件路径（如果指定的话）
	// 这需要在 logger.InitLogger() 之前完成，因为配置初始化发生在 init() 中
	// 注意：flag.Parse() 会在 flagParse() 中调用，所以需要手动解析 config 标志
	for i, arg := range os.Args {
		if strings.HasPrefix(arg, "--config=") {
			configPath = arg[9:] // 去掉 "--config=" 前缀
			break
		} else if arg == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
			break
		}
	}

	// 尝试从文件系统读取本地配置文件
	bytes, err = os.ReadFile(configPath)
	if err != nil {
		logger.Debug("Read config.yaml from file system failed, using embedded config. Error: " + err.Error())
		bytes, err = embeddedConfig.ReadFile("config.yaml")
		if err != nil {
			logger.Failedf("Can not read embedded config file: %v", err)
		}
	}

	if err := yaml.Unmarshal(bytes, &ConfigData); err != nil {
		logger.Failedf("Failed to parse config file: %v", err)
	}

	// Set defaults if not specified
	if ConfigData.Server.Port == 0 {
		ConfigData.Server.Port = 53317
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
