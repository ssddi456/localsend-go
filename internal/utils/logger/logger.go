// logger/logger.go
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows/svc"
)

type Logger struct {
	*logrus.Logger
}

// LogConfig 定义日志配置选项
type LogConfig struct {
	Level        logrus.Level
	Output       io.Writer
	Formatter    logrus.Formatter
	ReportCaller bool
}

var (
	logger *Logger
	once   sync.Once

	// ANSI 颜色代码
	green = "\033[32m"
	red   = "\033[31m"
	reset = "\033[0m"
)

// DefaultConfig 返回默认配置
func DefaultConfig() LogConfig {
	// 检测是否在 Windows 服务环境中运行
	isService, err := svc.IsWindowsService()
	output := io.Writer(os.Stdout)
	formatter := &logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	}

	// 如果在 svc 中运行，输出到文件而不是 stdout
	if err == nil && isService {
		logFile, err := getServiceLogFile()
		if err == nil {
			output = logFile
			// 服务模式下禁用颜色
			formatter = &logrus.TextFormatter{
				FullTimestamp: true,
				ForceColors:   false,
			}
		}
	}

	return LogConfig{
		Level:        logrus.InfoLevel,
		Output:       output,
		Formatter:    formatter,
		ReportCaller: false,
	}
}

// getServiceLogFile 获取服务日志文件
func getServiceLogFile() (io.Writer, error) {
	// 尝试在程序所在目录创建 logs 目录
	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	logDir := filepath.Join(filepath.Dir(exePath), "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}

	logFilePath := filepath.Join(logDir, "localsend-service.log")
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// InitLogger 初始化 Logger
func InitLogger(config ...LogConfig) {
	once.Do(func() {
		cfg := DefaultConfig()
		if len(config) > 0 {
			cfg = config[0]
		}

		log := logrus.New()
		log.SetOutput(cfg.Output)
		log.SetFormatter(cfg.Formatter)
		log.SetLevel(cfg.Level)
		log.SetReportCaller(cfg.ReportCaller)

		logger = &Logger{log}
	})
}

// checkLogger 确保 logger 已初始化
func checkLogger() {
	if logger == nil {
		InitLogger() // 使用默认配置初始化
	}
}

// GetLogger 返回底层 Logger 实例
func GetLogger() *Logger {
	checkLogger()
	return logger
}

// Success 打印带有绿色 [Success] 标签的信息
func Success(args ...interface{}) {
	checkLogger()
	logger.Infof("%s[Success]%s %s", green, reset, fmt.Sprint(args...))
}

// Successf 打印带有绿色 [Success] 标签的格式化信息
func Successf(format string, args ...interface{}) {
	checkLogger()
	logger.Infof("%s[Success]%s %s", green, reset, fmt.Sprintf(format, args...))
}

// Failed 打印带有红色 [Failed] 标签的信息
func Failed(args ...interface{}) {
	checkLogger()
	logger.Errorf("%s[Failed]%s %s", red, reset, fmt.Sprint(args...))
}

// Failedf 打印带有红色 [Failed] 标签的格式化信息
func Failedf(format string, args ...interface{}) {
	checkLogger()
	logger.Errorf("%s[Failed]%s %s", red, reset, fmt.Sprintf(format, args...))
}

func Debug(args ...interface{}) {
	checkLogger()
	logger.Debug(args...)
}

func Debugf(format string, args ...interface{}) {
	checkLogger()
	logger.Debugf(format, args...)
}

// Info 打印信息级别日志
func Info(args ...interface{}) {
	checkLogger()
	logger.Info(args...)
}

// Infof 打印信息级别日志（支持格式化）
func Infof(format string, args ...interface{}) {
	checkLogger()
	logger.Infof(format, args...)
}

// Warn 打印警告级别日志
func Warn(args ...interface{}) {
	checkLogger()
	logger.Warn(args...)
}

// Warnf 打印警告级别日志（支持格式化）
func Warnf(format string, args ...interface{}) {
	checkLogger()
	logger.Warnf(format, args...)
}

// Error 打印错误级别日志
func Error(args ...interface{}) {
	checkLogger()
	logger.Error(args...)
}

// Errorf 打印错误级别日志（支持格式化）
func Errorf(format string, args ...interface{}) {
	checkLogger()
	logger.Errorf(format, args...)
}

// WithFields 支持结构化日志
func WithFields(fields logrus.Fields) *Logger {
	checkLogger()
	return &Logger{logger.WithFields(fields).Logger}
}
