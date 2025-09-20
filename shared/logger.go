package shared

import (
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logger     *zap.Logger
	loggerOnce sync.Once
)

// LoggerConfig holds the configuration for the logger
type LoggerConfig struct {
	// ServerName identifies the server instance
	ServerName string

	// Environment (development, production, staging)
	Environment string

	// LogLevel defines the minimum log level
	LogLevel string

	// EnableConsole enables console output
	EnableConsole bool

	// EnableFile enables file output
	EnableFile bool

	// LogFilePath is the path to the log file
	LogFilePath string

	// EnableJSON enables JSON format output
	EnableJSON bool

	// EnableCaller enables caller information in logs
	EnableCaller bool

	// EnableStacktrace enables stacktrace for error level and above
	EnableStacktrace bool
}

// DefaultLoggerConfig returns a sensible default configuration
func DefaultLoggerConfig() *LoggerConfig {
	serverName := os.Getenv("SERVER_NAME")
	if serverName == "" {
		serverName = "unknown-server"
	}

	environment := os.Getenv("ENVIRONMENT")
	if environment == "" {
		environment = "development"
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		if environment == "production" {
			logLevel = "info"
		} else {
			logLevel = "debug"
		}
	}

	return &LoggerConfig{
		ServerName:       serverName,
		Environment:      environment,
		LogLevel:         logLevel,
		EnableConsole:    true,
		EnableFile:       environment == "production",
		LogFilePath:      "/var/log/app.log",
		EnableJSON:       environment == "production",
		EnableCaller:     environment != "production",
		EnableStacktrace: true,
	}
}

// InitLogger initializes the global logger with the provided configuration
func InitLogger(config *LoggerConfig) error {
	var err error
	loggerOnce.Do(func() {
		logger, err = createLogger(config)
	})

	// Replace the global zap logger
	if err == nil {
		zap.ReplaceGlobals(logger)
	}

	return err
}

// GetLogger returns the global logger instance
func GetLogger() *zap.Logger {
	if logger == nil {
		// Initialize with default config if not already initialized
		config := DefaultLoggerConfig()
		if err := InitLogger(config); err != nil {
			// Fallback to a simple production logger
			logger = zap.Must(zap.NewProduction())
		}
	}
	return logger
}

// GetSugaredLogger returns a sugared logger for easier usage
func GetSugaredLogger() *zap.SugaredLogger {
	return GetLogger().Sugar()
}

// createLogger creates a new zap logger with the specified configuration
func createLogger(config *LoggerConfig) (*zap.Logger, error) {
	// Parse log level
	level := parseLogLevel(config.LogLevel)

	// Create encoder config
	encoderConfig := createEncoderConfig(config)

	// Create cores for different outputs
	var cores []zapcore.Core

	// Console output
	if config.EnableConsole {
		consoleEncoder := createConsoleEncoder(config, encoderConfig)
		consoleWriter := zapcore.Lock(os.Stdout)
		consoleCore := zapcore.NewCore(consoleEncoder, consoleWriter, level)
		cores = append(cores, consoleCore)
	}

	// File output
	if config.EnableFile && config.LogFilePath != "" {
		fileEncoder := createFileEncoder(config, encoderConfig)
		if fileWriter, err := createFileWriter(config.LogFilePath); err == nil {
			fileCore := zapcore.NewCore(fileEncoder, fileWriter, level)
			cores = append(cores, fileCore)
		}
	}

	// Combine cores
	core := zapcore.NewTee(cores...)

	// Create logger options
	opts := []zap.Option{
		zap.AddCallerSkip(1),
	}

	if config.EnableCaller {
		opts = append(opts, zap.AddCaller())
	}

	if config.EnableStacktrace {
		opts = append(opts, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	// Create base logger
	baseLogger := zap.New(core, opts...)

	// Add server name to all logs
	logger := baseLogger.With(
		zap.String("server_name", config.ServerName),
		zap.String("environment", config.Environment),
	)

	return logger, nil
}

// parseLogLevel converts string log level to zapcore.Level
func parseLogLevel(levelStr string) zapcore.Level {
	switch strings.ToLower(levelStr) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "fatal":
		return zapcore.FatalLevel
	case "panic":
		return zapcore.PanicLevel
	default:
		return zapcore.InfoLevel
	}
}

// createEncoderConfig creates the base encoder configuration
func createEncoderConfig(config *LoggerConfig) zapcore.EncoderConfig {
	encoderConfig := zap.NewProductionEncoderConfig()

	// Customize time format
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// Customize level format
	encoderConfig.LevelKey = "level"
	encoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder

	// Customize caller format
	if config.EnableCaller {
		encoderConfig.CallerKey = "caller"
		encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	}

	// Message and name keys
	encoderConfig.MessageKey = "message"
	encoderConfig.NameKey = "logger"

	return encoderConfig
}

// createConsoleEncoder creates an encoder for console output
func createConsoleEncoder(config *LoggerConfig, encoderConfig zapcore.EncoderConfig) zapcore.Encoder {
	if config.EnableJSON {
		return zapcore.NewJSONEncoder(encoderConfig)
	}

	// For development, use colored console output
	if config.Environment == "development" {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	return zapcore.NewConsoleEncoder(encoderConfig)
}

// createFileEncoder creates an encoder for file output
func createFileEncoder(config *LoggerConfig, encoderConfig zapcore.EncoderConfig) zapcore.Encoder {
	// File output should always be JSON for better parsing
	return zapcore.NewJSONEncoder(encoderConfig)
}

// createFileWriter creates a file writer with rotation if available
func createFileWriter(filePath string) (zapcore.WriteSyncer, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return zapcore.AddSync(file), nil
}

// LogWithServerContext adds server context to existing logger
func LogWithServerContext(logger *zap.Logger, serverName string) *zap.Logger {
	return logger.With(
		zap.String("server_name", serverName),
		zap.String("pid", os.Getenv("PID")),
	)
}

// Helper functions for common logging patterns

// LogStartup logs server startup information
func LogStartup(serverName, version, port string) {
	GetLogger().Info("Server starting up",
		zap.String("server_name", serverName),
		zap.String("version", version),
		zap.String("port", port),
		zap.String("pid", os.Getenv("PID")),
	)
}

// LogShutdown logs server shutdown information
func LogShutdown(serverName string, reason string) {
	GetLogger().Info("Server shutting down",
		zap.String("server_name", serverName),
		zap.String("reason", reason),
	)
}

// Sync flushes any buffered log entries
func Sync() {
	if logger != nil {
		_ = logger.Sync()
	}
}
