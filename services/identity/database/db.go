package database

import (
	"errors"

	"github.com/gabehamasaki/momentum/services/identity/models"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Database struct {
	DSN string
}

func NewDB(DSN string) *Database {
	return &Database{
		DSN: DSN,
	}
}

func (d *Database) Conn() (*gorm.DB, error) {
	return gorm.Open(postgres.Open(d.DSN), &gorm.Config{})
}

func (d *Database) Migrate() error {
	db, err := d.Conn()
	if err != nil {
		return err
	}

	return db.AutoMigrate(&models.Permission{}, &models.Role{}, &models.User{})
}

func (d *Database) Seeder() error {
	roles := []models.Role{
		{
			ID:   uuid.New().String(),
			Name: "member",
			Permissions: []*models.Permission{
				{Name: "profile.edit"},
				{Name: "profile.view"},
			},
		},
		{
			ID:   uuid.New().String(),
			Name: "admin",
			Permissions: []*models.Permission{
				{Name: "profile.edit"},
				{Name: "profile.view"},
				{Name: "user.view"},
				{Name: "user.delete"},
				{Name: "user.store"},
				{Name: "user.update"},
			},
		},
	}

	db, err := d.Conn()
	if err != nil {
		return err
	}

	for _, role := range roles {
		var existingRole models.Role
		if err := db.Where("name = ?", role.Name).First(&existingRole).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := db.Create(&role).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	return nil
}
