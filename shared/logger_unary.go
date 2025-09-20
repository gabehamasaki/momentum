package shared

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// InterceptorConfig configures the logging interceptor behavior
type InterceptorConfig struct {
	// Logger is the zap logger to use (defaults to global logger)
	Logger *zap.Logger

	// LogLevel defines the log level (defaults to Info)
	LogLevel zapcore.Level

	// LogRequests enables request payload logging (defaults to true)
	LogRequests bool

	// LogResponses enables response payload logging (defaults to false for security)
	LogResponses bool

	// LogMetadata enables gRPC metadata logging (defaults to false)
	LogMetadata bool

	// SensitiveFields are field names that should be redacted in logs
	SensitiveFields []string

	// SlowRequestThreshold logs a warning for requests taking longer than this duration
	SlowRequestThreshold time.Duration

	// ServerName is added to all log entries to identify the server
	ServerName string
}

// DefaultInterceptorConfig returns a sensible default configuration
func DefaultInterceptorConfig() *InterceptorConfig {
	serverName := os.Getenv("SERVER_NAME")
	if serverName == "" {
		serverName = "unknown-server"
	}

	return &InterceptorConfig{
		Logger:               GetLogger(),
		LogLevel:             zapcore.InfoLevel,
		LogRequests:          true,
		LogResponses:         false, // Disabled by default for security
		LogMetadata:          false,
		SensitiveFields:      []string{"password", "token", "secret", "key", "authorization"},
		SlowRequestThreshold: 5 * time.Second,
		ServerName:           serverName,
	}
}

// LoggingUnaryInterceptor creates a unary server interceptor with enhanced logging capabilities
func LoggingUnaryInterceptor(config *InterceptorConfig) grpc.UnaryServerInterceptor {
	if config == nil {
		config = DefaultInterceptorConfig()
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		startTime := time.Now()

		// Create base logger with method info
		logger := config.Logger.With(
			zap.String("server_name", config.ServerName),
			zap.String("grpc.method", info.FullMethod),
			zap.String("grpc.service", extractServiceName(info.FullMethod)),
			zap.String("grpc.start_time", startTime.UTC().Format(time.RFC3339)),
		)

		// Add client info if available
		if p, ok := peer.FromContext(ctx); ok {
			logger = logger.With(zap.String("grpc.peer.addr", p.Addr.String()))
		}

		// Add metadata if enabled
		if config.LogMetadata {
			if md, ok := metadata.FromIncomingContext(ctx); ok {
				logger = logger.With(zap.Any("grpc.metadata", sanitizeMetadata(md, config.SensitiveFields)))
			}
		}

		// Log incoming request
		if config.LogRequests {
			sanitizedReq := sanitizeFields(req, config.SensitiveFields)
			logger.Log(config.LogLevel, "gRPC request received",
				zap.Any("grpc.request", sanitizedReq),
			)
		} else {
			logger.Log(config.LogLevel, "gRPC request received")
		}

		// Handle panic recovery
		defer func() {
			if r := recover(); r != nil {
				err = status.Errorf(codes.Internal, "panic recovered: %v", r)
				logger.Error("gRPC method panicked",
					zap.Any("grpc.panic", r),
					zap.String("grpc.stack", string(debug.Stack())),
					zap.Duration("grpc.duration", time.Since(startTime)),
				)
			}
		}()

		// Call the handler
		resp, err = handler(ctx, req)
		duration := time.Since(startTime)

		// Prepare log fields
		logFields := []zap.Field{
			zap.Duration("grpc.duration", duration),
			zap.String("grpc.code", status.Code(err).String()),
		}

		if err != nil {
			// Log error details
			st, _ := status.FromError(err)
			logger.Error("gRPC method failed",
				append(logFields,
					zap.Error(err),
					zap.String("grpc.status", st.Code().String()),
					zap.String("grpc.message", st.Message()),
				)...,
			)
		} else {
			// Log successful completion
			if config.LogResponses && resp != nil {
				sanitizedResp := sanitizeFields(resp, config.SensitiveFields)
				logFields = append(logFields, zap.Any("grpc.response", sanitizedResp))
			}

			// Check for slow requests
			if duration > config.SlowRequestThreshold {
				logFields = append(logFields, zap.Bool("grpc.slow_request", true))
				logger.Warn("gRPC method completed (SLOW)", logFields...)
			} else {
				logger.Log(config.LogLevel, "gRPC method completed", logFields...)
			}
		}

		return resp, err
	}
}

// extractServiceName extracts the service name from the full method path
func extractServiceName(fullMethod string) string {
	parts := strings.Split(fullMethod, "/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return fullMethod
}

// sanitizeMetadata removes sensitive data from gRPC metadata
func sanitizeMetadata(md metadata.MD, sensitiveFields []string) map[string][]string {
	sanitized := make(map[string][]string)

	for key, values := range md {
		lowerKey := strings.ToLower(key)
		isSensitive := false

		for _, field := range sensitiveFields {
			if strings.Contains(lowerKey, strings.ToLower(field)) {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			sanitized[key] = []string{"[REDACTED]"}
		} else {
			sanitized[key] = values
		}
	}

	return sanitized
}

// sanitizeFields recursively removes sensitive data from structs
func sanitizeFields(obj any, sensitiveFields []string) any {
	if obj == nil {
		return nil
	}

	// For now, convert to string and check for sensitive patterns
	// In a real implementation, you might want to use reflection
	// to properly handle struct fields
	objStr := fmt.Sprintf("%+v", obj)

	// Simple sanitization - replace potential sensitive values
	for _, field := range sensitiveFields {
		if strings.Contains(strings.ToLower(objStr), strings.ToLower(field)) {
			return "[REDACTED - Contains sensitive data]"
		}
	}

	return obj
}

// Simple usage function for backward compatibility
func LogInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	config := DefaultInterceptorConfig()
	interceptor := LoggingUnaryInterceptor(config)
	return interceptor(ctx, req, info, handler)
}
