package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	FATAL
)

func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
)

type Logger struct {
	mu         sync.Mutex
	out        io.Writer
	level      LogLevel
	prefix     string
	colorize   bool
	showCaller bool
	showTime   bool
	timeFormat string
	stdLogger  *log.Logger
}

var (
	defaultLogger *Logger
	once          sync.Once
)

type Config struct {
	Level      LogLevel
	Prefix     string
	Colorize   bool
	ShowCaller bool
	ShowTime   bool
	TimeFormat string
	Output     io.Writer
}

func DefaultConfig() Config {
	return Config{
		Level:      INFO,
		Prefix:     "",
		Colorize:   true,
		ShowCaller: false,
		ShowTime:   true,
		TimeFormat: "2006-01-02 15:04:05",
		Output:     os.Stdout,
	}
}

func New(cfg Config) *Logger {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	if cfg.TimeFormat == "" {
		cfg.TimeFormat = "2006-01-02 15:04:05"
	}

	return &Logger{
		out:        cfg.Output,
		level:      cfg.Level,
		prefix:     cfg.Prefix,
		colorize:   cfg.Colorize,
		showCaller: cfg.ShowCaller,
		showTime:   cfg.ShowTime,
		timeFormat: cfg.TimeFormat,
		stdLogger:  log.New(cfg.Output, cfg.Prefix, 0),
	}
}

func GetLogger() *Logger {
	once.Do(func() {
		cfg := DefaultConfig()
		if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
			switch strings.ToUpper(envLevel) {
			case "DEBUG":
				cfg.Level = DEBUG
			case "INFO":
				cfg.Level = INFO
			case "WARN":
				cfg.Level = WARN
			case "FATAL":
				cfg.Level = FATAL
			}
		}
		defaultLogger = New(cfg)
	})
	return defaultLogger
}

func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out = w
	l.stdLogger.SetOutput(w)
}

func (l *Logger) SetColorize(colorize bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.colorize = colorize
}

func (l *Logger) SetShowCaller(show bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.showCaller = show
}

func (l *Logger) formatMessage(level LogLevel, msg string, args ...any) string {
	var parts []string

	if l.showTime {
		timestamp := time.Now().Format(l.timeFormat)
		parts = append(parts, timestamp)
	}

	levelStr := fmt.Sprintf("[%s]", level.String())
	if l.colorize {
		switch level {
		case DEBUG:
			levelStr = colorGray + levelStr + colorReset
		case INFO:
			levelStr = colorBlue + levelStr + colorReset
		case WARN:
			levelStr = colorYellow + levelStr + colorReset
		case FATAL:
			levelStr = colorRed + levelStr + colorReset
		}
	}
	parts = append(parts, levelStr)

	if l.showCaller {
		if _, file, line, ok := runtime.Caller(3); ok {
			// Get just the filename, not the full path
			idx := strings.LastIndex(file, "/")
			if idx >= 0 {
				file = file[idx+1:]
			}
			caller := fmt.Sprintf("%s:%d", file, line)
			parts = append(parts, caller)
		}
	}

	if l.prefix != "" {
		parts = append(parts, l.prefix)
	}

	var message string
	if len(args) > 0 {
		message = fmt.Sprintf(msg, args...)
	} else {
		message = msg
	}
	parts = append(parts, message)

	return strings.Join(parts, " ")
}

// log is the internal logging method
func (l *Logger) log(level LogLevel, msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	formattedMsg := l.formatMessage(level, msg, args...)
	fmt.Fprintln(l.out, formattedMsg)

	if level == FATAL {
		os.Exit(1)
	}
}

// Debug logs a message at DEBUG level
func (l *Logger) Debug(msg string, args ...any) {
	l.log(DEBUG, msg, args...)
}

// Info logs a message at INFO level
func (l *Logger) Info(msg string, args ...any) {
	l.log(INFO, msg, args...)
}

// Warn logs a message at WARN level
func (l *Logger) Warn(msg string, args ...any) {
	l.log(WARN, msg, args...)
}

// Fatal logs a message at FATAL level and exits the program
func (l *Logger) Fatal(msg string, args ...any) {
	l.log(FATAL, msg, args...)
}

// Error is an alias for Warn for backwards compatibility
func (l *Logger) Error(msg string, args ...any) {
	l.log(WARN, msg, args...)
}

// Debugf logs a formatted message at DEBUG level
func (l *Logger) Debugf(format string, args ...any) {
	l.Debug(format, args...)
}

// Infof logs a formatted message at INFO level
func (l *Logger) Infof(format string, args ...any) {
	l.Info(format, args...)
}

// Warnf logs a formatted message at WARN level
func (l *Logger) Warnf(format string, args ...any) {
	l.Warn(format, args...)
}

// Fatalf logs a formatted message at FATAL level and exits
func (l *Logger) Fatalf(format string, args ...any) {
	l.Fatal(format, args...)
}

// Errorf is an alias for Warnf
func (l *Logger) Errorf(format string, args ...any) {
	l.Warnf(format, args...)
}

// Package-level convenience functions using the default logger

// Debug logs a debug message using the default logger
func Debug(msg string, args ...any) {
	GetLogger().Debug(msg, args...)
}

// Info logs an info message using the default logger
func Info(msg string, args ...any) {
	GetLogger().Info(msg, args...)
}

// Warn logs a warning message using the default logger
func Warn(msg string, args ...any) {
	GetLogger().Warn(msg, args...)
}

// Fatal logs a fatal message and exits using the default logger
func Fatal(msg string, args ...any) {
	GetLogger().Fatal(msg, args...)
}

// Error is an alias for Warn using the default logger
func Error(msg string, args ...any) {
	GetLogger().Error(msg, args...)
}

// Debugf logs a formatted debug message using the default logger
func Debugf(format string, args ...any) {
	GetLogger().Debugf(format, args...)
}

// Infof logs a formatted info message using the default logger
func Infof(format string, args ...any) {
	GetLogger().Infof(format, args...)
}

// Warnf logs a formatted warning message using the default logger
func Warnf(format string, args ...any) {
	GetLogger().Warnf(format, args...)
}

// Fatalf logs a formatted fatal message and exits using the default logger
func Fatalf(format string, args ...any) {
	GetLogger().Fatalf(format, args...)
}

// Errorf is an alias for Warnf using the default logger
func Errorf(format string, args ...any) {
	GetLogger().Errorf(format, args...)
}

// SetLevel sets the log level for the default logger
func SetLevel(level LogLevel) {
	GetLogger().SetLevel(level)
}

// SetOutput sets the output for the default logger
func SetOutput(w io.Writer) {
	GetLogger().SetOutput(w)
}

// SetColorize enables or disables colored output for the default logger
func SetColorize(colorize bool) {
	GetLogger().SetColorize(colorize)
}

// SetShowCaller enables or disables caller information for the default logger
func SetShowCaller(show bool) {
	GetLogger().SetShowCaller(show)
}
