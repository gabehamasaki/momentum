package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gabehamasaki/momentum/services/identity/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Database representa a configuração e conexão com o banco de dados
type Database struct {
	DSN        string
	connection *gorm.DB
	mu         sync.RWMutex
	config     *DatabaseConfig
}

// DatabaseConfig contém configurações para o banco de dados
type DatabaseConfig struct {
	MaxOpenConnections    int
	MaxIdleConnections    int
	ConnectionMaxLifetime time.Duration
	ConnectionMaxIdleTime time.Duration
	LogLevel              logger.LogLevel
	SlowQueryThreshold    time.Duration
}

// DatabaseStats contém estatísticas da conexão com o banco de dados
type DatabaseStats struct {
	OpenConnections int
	InUse           int
	Idle            int
}

// DefaultDatabaseConfig retorna uma configuração padrão otimizada
func DefaultDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		MaxOpenConnections:    25,
		MaxIdleConnections:    5,
		ConnectionMaxLifetime: 5 * time.Minute,
		ConnectionMaxIdleTime: 1 * time.Minute,
		LogLevel:              logger.Silent,
		SlowQueryThreshold:    200 * time.Millisecond,
	}
}

// NewDB cria uma nova instância de Database com configuração padrão
func NewDB(DSN string) *Database {
	return NewDBWithConfig(DSN, DefaultDatabaseConfig())
}

// NewDBWithConfig cria uma nova instância de Database com configuração customizada
func NewDBWithConfig(DSN string, config *DatabaseConfig) *Database {
	if config == nil {
		config = DefaultDatabaseConfig()
	}
	return &Database{
		DSN:    DSN,
		config: config,
	}
}

// Conn retorna a conexão com o banco de dados, criando uma nova se necessário
func (d *Database) Conn() (*gorm.DB, error) {
	return d.ConnWithContext(context.Background())
}

// ConnWithContext retorna a conexão com o banco de dados com contexto
func (d *Database) ConnWithContext(ctx context.Context) (*gorm.DB, error) {
	d.mu.RLock()
	if d.connection != nil {
		defer d.mu.RUnlock()
		return d.connection, nil
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()

	// Double-check locking pattern
	if d.connection != nil {
		return d.connection, nil
	}

	return d.createConnection(ctx)
}

// createConnection cria uma nova conexão com o banco de dados
func (d *Database) createConnection(ctx context.Context) (*gorm.DB, error) {
	if d.DSN == "" {
		return nil, errors.New("DSN não pode estar vazio")
	}

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(d.config.LogLevel),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	db, err := gorm.Open(postgres.Open(d.DSN), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar ao banco de dados: %w", err)
	}

	// Configurar pool de conexões
	if err := d.configureConnectionPool(db); err != nil {
		return nil, fmt.Errorf("falha ao configurar pool de conexões: %w", err)
	}

	// Testar conexão
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("falha ao obter conexão SQL: %w", err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("falha ao fazer ping no banco de dados: %w", err)
	}

	d.connection = db
	return d.connection, nil
}

// configureConnectionPool configura o pool de conexões do banco de dados
func (d *Database) configureConnectionPool(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	sqlDB.SetMaxOpenConns(d.config.MaxOpenConnections)
	sqlDB.SetMaxIdleConns(d.config.MaxIdleConnections)
	sqlDB.SetConnMaxLifetime(d.config.ConnectionMaxLifetime)
	sqlDB.SetConnMaxIdleTime(d.config.ConnectionMaxIdleTime)

	return nil
}

// Close fecha a conexão com o banco de dados
func (d *Database) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.connection == nil {
		return nil
	}

	sqlDB, err := d.connection.DB()
	if err != nil {
		return fmt.Errorf("falha ao obter conexão SQL para fechamento: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("falha ao fechar conexão com banco de dados: %w", err)
	}

	d.connection = nil
	return nil
}

// Migrate executa as migrações do banco de dados
func (d *Database) Migrate() error {
	return d.MigrateWithContext(context.Background())
}

// MigrateWithContext executa as migrações do banco de dados com contexto
func (d *Database) MigrateWithContext(ctx context.Context) error {
	db, err := d.ConnWithContext(ctx)
	if err != nil {
		return fmt.Errorf("falha ao conectar para migração: %w", err)
	}

	// Lista de modelos para migrar
	models := []interface{}{
		&models.Permission{},
		&models.Role{},
		&models.User{},
	}

	for _, model := range models {
		if err := db.WithContext(ctx).AutoMigrate(model); err != nil {
			return fmt.Errorf("falha ao migrar modelo %T: %w", model, err)
		}
	}

	return nil
}

// Seeder popula o banco de dados com dados iniciais
func (d *Database) Seeder() error {
	return d.SeederWithContext(context.Background())
}

// SeederWithContext popula o banco de dados com dados iniciais com contexto
func (d *Database) SeederWithContext(ctx context.Context) error {
	db, err := d.ConnWithContext(ctx)
	if err != nil {
		return fmt.Errorf("falha ao conectar para seed: %w", err)
	}

	// Executar seed em transação para garantir consistência
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := d.seedPermissions(tx); err != nil {
			return fmt.Errorf("falha ao fazer seed das permissões: %w", err)
		}

		if err := d.seedRoles(tx); err != nil {
			return fmt.Errorf("falha ao fazer seed das roles: %w", err)
		}

		return nil
	})
}

// seedPermissions cria as permissões iniciais
func (d *Database) seedPermissions(tx *gorm.DB) error {
	permissions := []string{
		"profile.edit",
		"profile.view",
		"user.view",
		"user.delete",
		"user.store",
		"user.update",
	}

	log.Printf("Criando %d permissões...", len(permissions))

	for _, name := range permissions {
		var perm models.Permission
		err := tx.Where("name = ?", name).First(&perm).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			perm = models.Permission{Name: name}
			if err := tx.Create(&perm).Error; err != nil {
				return fmt.Errorf("falha ao criar permissão '%s': %w", name, err)
			}
		} else if err != nil {
			return fmt.Errorf("falha ao buscar permissão '%s': %w", name, err)
		}
	}

	return nil
}

// seedRoles cria as roles e associa as permissões
func (d *Database) seedRoles(tx *gorm.DB) error {
	rolesConfig := map[string][]string{
		"member": {"profile.edit", "profile.view"},
		"admin": {
			"profile.edit", "profile.view",
			"user.view", "user.delete", "user.store", "user.update",
		},
	}

	for roleName, permNames := range rolesConfig {
		if err := d.createRoleWithPermissions(tx, roleName, permNames); err != nil {
			return fmt.Errorf("falha ao criar role '%s': %w", roleName, err)
		}
	}

	return nil
}

// createRoleWithPermissions cria uma role e associa suas permissões
func (d *Database) createRoleWithPermissions(tx *gorm.DB, roleName string, permNames []string) error {
	// Verificar se a role já existe
	var role models.Role
	err := tx.Where("name = ?", roleName).First(&role).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Criar nova role
		role = models.Role{
			Name: roleName,
		}
		if err := tx.Create(&role).Error; err != nil {
			return fmt.Errorf("falha ao criar role: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("falha ao buscar role: %w", err)
	}

	// Buscar permissões em lote
	var permissions []models.Permission
	if err := tx.Where("name IN ?", permNames).Find(&permissions).Error; err != nil {
		return fmt.Errorf("falha ao buscar permissões: %w", err)
	}

	if len(permissions) != len(permNames) {
		return fmt.Errorf("nem todas as permissões foram encontradas para a role '%s'", roleName)
	}

	// Limpar associações existentes para evitar duplicatas
	if err := tx.Model(&role).Association("Permissions").Clear(); err != nil {
		return fmt.Errorf("falha ao limpar permissões existentes: %w", err)
	}

	// Associar permissões à role
	if err := tx.Model(&role).Association("Permissions").Append(permissions); err != nil {
		return fmt.Errorf("falha ao associar permissões: %w", err)
	}

	return nil
}

// HealthCheck verifica se o banco de dados está saudável
func (d *Database) HealthCheck(ctx context.Context) error {
	db, err := d.ConnWithContext(ctx)
	if err != nil {
		return fmt.Errorf("falha na conexão: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("falha ao obter conexão SQL: %w", err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("falha no ping: %w", err)
	}

	return nil
}

// Stats retorna estatísticas da conexão com o banco de dados
func (d *Database) Stats() (*DatabaseStats, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.connection == nil {
		return nil, errors.New("conexão não estabelecida")
	}

	sqlDB, err := d.connection.DB()
	if err != nil {
		return nil, fmt.Errorf("falha ao obter conexão SQL: %w", err)
	}

	stats := sqlDB.Stats()
	return &DatabaseStats{
		OpenConnections: stats.OpenConnections,
		InUse:           stats.InUse,
		Idle:            stats.Idle,
	}, nil
}
