package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/gabehamasaki/momentum/services/identity/database"
	"github.com/gabehamasaki/momentum/services/identity/server"
	"github.com/gabehamasaki/momentum/services/identity/services"
	"github.com/gabehamasaki/momentum/shared/v1/proto"
	_ "github.com/joho/godotenv/autoload"
	"google.golang.org/grpc"
)

func main() {
	// Configuração de timeout para operações de database
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verificar se DSN está definida
	dsn := os.Getenv("IDENTITY_DSN")
	if dsn == "" {
		log.Fatalf("Variável de ambiente IDENTITY_DSN não está definida")
	}

	// Inicializar database
	log.Println("Inicializando database...")
	config := database.DefaultDatabaseConfig()
	// Exemplo de customização se necessário:
	// config.MaxOpenConnections = 50

	db := database.NewDBWithConfig(dsn, config)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("Erro ao fechar conexão com database: %v", closeErr)
		}
	}()

	// Conectar ao database com retry
	log.Println("Conectando ao database...")
	const maxRetries = 3
	const retryDelay = 2 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			log.Fatalf("Timeout na conexão com database: %v", ctx.Err())
		default:
		}

		_, err := db.ConnWithContext(ctx)
		if err == nil {
			// Fazer health check
			if healthErr := db.HealthCheck(ctx); healthErr == nil {
				log.Println("✓ Conectado ao database com sucesso")

				// Mostrar estatísticas da conexão
				if stats, statsErr := db.Stats(); statsErr == nil {
					log.Printf("✓ Pool de conexões: %d aberta(s), %d em uso, %d idle",
						stats.OpenConnections, stats.InUse, stats.Idle)
				}
				break
			} else {
				err = healthErr
			}
		}

		if attempt == maxRetries {
			log.Fatalf("Falha na conexão após %d tentativas: %v", maxRetries, err)
		}

		log.Printf("Tentativa %d/%d falhou, tentando novamente em %v... Erro: %v",
			attempt, maxRetries, retryDelay, err)
		time.Sleep(retryDelay)
	}

	// Executar migrations
	log.Println("Executando migrations...")
	if err := db.MigrateWithContext(ctx); err != nil {
		log.Fatalf("Falha ao executar migrations: %v", err)
	}
	// Fazer seed do database
	log.Println("Populando database com dados iniciais...")
	if err := db.SeederWithContext(ctx); err != nil {
		log.Fatalf("Falha ao popular database: %v", err)
	}
	log.Println("Sistema inicializado com sucesso!")

	// Iniciar servidor gRPC
	log.Println("Iniciando servidor gRPC...")

	l, err := net.Listen("tcp", fmt.Sprintf(":%s", os.Getenv("IDENTITY_GRPC_PORT")))
	if err != nil {
		log.Fatalf("Falha ao iniciar listener: %v", err)
	}

	grpcServer := grpc.NewServer()

	userService := services.NewUserService(db)

	proto.RegisterIdentityServiceServer(grpcServer, server.NewIdentityServer(userService))

	log.Printf("Servidor gRPC rodando na porta %s", os.Getenv("IDENTITY_GRPC_PORT"))
	if serveErr := grpcServer.Serve(l); serveErr != nil {
		log.Fatalf("Falha ao rodar servidor gRPC: %v", serveErr)
	}
}

