package log

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
	"websearch/pkg/config"
)

var (
	defaultlog *zerolog.Logger
	fileWriter io.Closer
)

// NewLogger 同时输出到 stdout 控制台和滚动日志文件（HTTP daemon 使用）。
func NewLogger(logDir string, logConf config.LogConfig) *zerolog.Logger {
	return NewLoggerTo(os.Stdout, logDir, logConf)
}

// NewLoggerTo 将控制台日志写到 console（stdio CLI 应传 os.Stderr，避免污染 JSON-RPC）。
// console 为 nil 时不写控制台；logDir 为空时不写文件。
func NewLoggerTo(console io.Writer, logDir string, logConf config.LogConfig) *zerolog.Logger {
	_ = CloseFile()

	var writers []io.Writer
	if console != nil {
		writers = append(writers, zerolog.ConsoleWriter{
			Out:        console,
			TimeFormat: time.RFC3339,
		})
	}

	if logDir != "" {
		fw := &lumberjack.Logger{
			Filename:   filepath.Join(logDir, "websearch.log"),
			MaxSize:    logConf.MaxSize, // MB
			MaxAge:     logConf.MaxAge,  // days
			MaxBackups: 0,
			Compress:   false,
			LocalTime:  true,
		}
		fileWriter = fw
		writers = append(writers, fw)
	}

	if len(writers) == 0 {
		writers = append(writers, io.Discard)
	}

	multiWriter := zerolog.MultiLevelWriter(writers...)
	// CallerWithSkipFrameCount(3)：zerolog caller hook 自带 2 层内部帧，
	// 本仓库日志一律经包级 helper（Info/Infof/…）再进 zerolog，多 1 层；
	// 3 才能让 caller 指向真实调用点，1 会落进 zerolog 内部（event.go）。
	logger := zerolog.New(multiWriter).With().CallerWithSkipFrameCount(3).Timestamp().Logger()
	defaultlog = &logger
	return defaultlog
}

// CloseFile 关闭滚动日志文件句柄（Windows 上 TempDir/进程退出前需调用，否则文件会被占用）。
func CloseFile() error {
	if fileWriter == nil {
		return nil
	}
	err := fileWriter.Close()
	fileWriter = nil
	return err
}

func SetLoggerLevel(lv string) {
	if defaultlog == nil {
		return
	}
	level := zerolog.InfoLevel
	switch lv {
	case "DEBUG", "debug":
		level = zerolog.DebugLevel
	default:

	}
	defaultlog.Level(level)
}

func Debug(msg string) {
	if defaultlog != nil {
		defaultlog.Debug().Msg(msg)
	}
}

func Debugf(pattern string, v ...any) {
	if defaultlog != nil {
		defaultlog.Debug().Msgf(pattern, v...)
	}
}

func Info(msg string) {
	if defaultlog != nil {
		defaultlog.Info().Msg(msg)
	}
}

func Infof(pattern string, v ...any) {
	if defaultlog != nil {
		defaultlog.Info().Msgf(pattern, v...)
	}
}

func Warnf(pattern string, v ...any) {
	if defaultlog != nil {
		defaultlog.Warn().Msgf(pattern, v...)
	}
}

func Errf(pattern string, v ...any) {
	if defaultlog != nil {
		defaultlog.Error().Msgf(pattern, v...)
	}
}

func Error(msg string) {
	if defaultlog != nil {
		defaultlog.Error().Msg(msg)
	}
}
