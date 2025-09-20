package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gabehamasaki/momentum/services/identity/database"
	"github.com/gabehamasaki/momentum/services/identity/server"
	"github.com/gabehamasaki/momentum/services/identity/services"
	"github.com/gabehamasaki/momentum/shared"
	"github.com/gabehamasaki/momentum/shared/v1/proto"
	_ "github.com/joho/godotenv/autoload"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	serviceName    = "identity-service"
	serviceVersion = "v1.0.0"
)

func main() {
	// 1. Initialize logger first
	if err := initializeLogger(); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	logger := shared.GetLogger()

	// Log startup
	port := shared.GetEnv("IDENTITY_GRPC_PORT", "50051")
	shared.LogStartup(serviceName, serviceVersion, port)

	// 2. Setup graceful shutdown
	ctx, cancel := setupGracefulShutdown()
	defer cancel()

	// 3. Initialize database
	db, err := initializeDatabase(ctx, logger)
	if err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			logger.Error("Error closing database connection", zap.Error(closeErr))
		}
	}()

	// 4. Setup and start gRPC server
	grpcServer, listener := setupGRPCServer(logger, db, port)

	// Start server in goroutine
	go func() {
		logger.Info("Starting gRPC server",
			zap.String("address", listener.Addr().String()),
			zap.String("service", serviceName),
		)

		if err := grpcServer.Serve(listener); err != nil {
			logger.Fatal("Failed to serve gRPC", zap.Error(err))
		}
	}()

	// 5. Wait for shutdown signal
	<-ctx.Done()

	// 6. Graceful shutdown
	shared.LogShutdown(serviceName, "received shutdown signal")

	logger.Info("Shutting down gRPC server...")
	grpcServer.GracefulStop()

	logger.Info("Server shutdown completed")
	shared.Sync() // Flush logs
}

// initializeLogger sets up the zap logger with proper configuration
func initializeLogger() error {
	loggerConfig := &shared.LoggerConfig{
		ServerName:       serviceName,
		Environment:      shared.GetEnv("ENVIRONMENT", "development"),
		LogLevel:         shared.GetEnv("LOG_LEVEL", "info"),
		EnableConsole:    true,
		EnableFile:       shared.GetEnv("ENVIRONMENT", "development") == "production",
		LogFilePath:      "/var/log/identity-service.log",
		EnableJSON:       shared.GetEnv("ENVIRONMENT", "development") == "production",
		EnableCaller:     shared.GetEnv("ENVIRONMENT", "development") != "production",
		EnableStacktrace: true,
	}

	return shared.InitLogger(loggerConfig)
}

// setupGracefulShutdown configures graceful shutdown handling
func setupGracefulShutdown() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-c
		shared.GetLogger().Info("Received shutdown signal")
		cancel()
	}()

	return ctx, cancel
}

// initializeDatabase sets up database connection with retries and health checks
func initializeDatabase(ctx context.Context, logger *zap.Logger) (*database.Database, error) {
	// Get DSN from environment
	dsn := os.Getenv("IDENTITY_DSN")
	if dsn == "" {
		return nil, fmt.Errorf("IDENTITY_DSN environment variable is not set")
	}

	logger.Info("Initializing database connection")

	// Create database config
	config := database.DefaultDatabaseConfig()

	db := database.NewDBWithConfig(dsn, config)

	// Connect with retry logic
	const maxRetries = 3
	const retryDelay = 2 * time.Second

	logger.Info("Connecting to database with retry logic",
		zap.Int("max_retries", maxRetries),
		zap.Duration("retry_delay", retryDelay),
	)

	// Create a timeout context for database operations
	dbCtx, dbCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dbCancel()

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-dbCtx.Done():
			return nil, fmt.Errorf("database connection timeout: %w", dbCtx.Err())
		default:
		}

		// Attempt connection
		_, err := db.ConnWithContext(dbCtx)
		if err == nil {
			// Perform health check
			if healthErr := db.HealthCheck(dbCtx); healthErr == nil {
				logger.Info("Successfully connected to database")

				// Log connection stats
				if stats, statsErr := db.Stats(); statsErr == nil {
					logger.Info("Database connection pool status",
						zap.Int("open_connections", stats.OpenConnections),
						zap.Int("in_use", stats.InUse),
						zap.Int("idle", stats.Idle),
					)
				}
				break
			} else {
				lastErr = healthErr
			}
		} else {
			lastErr = err
		}

		if attempt == maxRetries {
			return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, lastErr)
		}

		logger.Warn("Database connection attempt failed, retrying",
			zap.Int("attempt", attempt),
			zap.Int("max_retries", maxRetries),
			zap.Duration("retry_in", retryDelay),
			zap.Error(lastErr),
		)

		time.Sleep(retryDelay)
	}

	// Run migrations
	logger.Info("Running database migrations")
	if err := db.MigrateWithContext(dbCtx); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Seed database
	logger.Info("Seeding database with initial data")
	if err := db.SeederWithContext(dbCtx); err != nil {
		return nil, fmt.Errorf("failed to seed database: %w", err)
	}

	logger.Info("Database initialization completed successfully")
	return db, nil
}

// setupGRPCServer creates and configures the gRPC server
func setupGRPCServer(logger *zap.Logger, db *database.Database, port string) (*grpc.Server, net.Listener) {
	// Configure interceptor
	interceptorConfig := &shared.InterceptorConfig{
		Logger:               logger,
		LogLevel:             zapcore.InfoLevel,
		LogRequests:          shared.GetEnv("LOG_GRPC_REQUESTS", "true") == "true",
		LogResponses:         shared.GetEnv("LOG_GRPC_RESPONSES", "false") == "true",
		LogMetadata:          shared.GetEnv("LOG_GRPC_METADATA", "false") == "true",
		SensitiveFields:      []string{"password", "token", "secret", "authorization", "cookie"},
		SlowRequestThreshold: 3 * time.Second,
		ServerName:           serviceName,
	}

	// Create gRPC server with interceptors
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(shared.LoggingUnaryInterceptor(interceptorConfig)),
	)

	// Initialize services
	logger.Info("Initializing services")
	userService := services.NewUserService(db)
	identityServer := server.NewIdentityServer(userService)

	// Register services
	proto.RegisterIdentityServiceServer(grpcServer, identityServer)

	// Enable reflection in development
	if shared.GetEnv("ENVIRONMENT", "development") == "development" {
		logger.Info("Enabling gRPC reflection for development")
		reflection.Register(grpcServer)
	}

	// Create listener
	address := fmt.Sprintf(":%s", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		logger.Fatal("Failed to create listener",
			zap.String("address", address),
			zap.Error(err),
		)
	}

	logger.Info("gRPC server configured",
		zap.String("address", listener.Addr().String()),
		zap.Bool("reflection_enabled", shared.GetEnv("ENVIRONMENT", "development") == "development"),
	)

	return grpcServer, listener
}
