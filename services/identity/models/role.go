package models

import (
	"time"

	"gorm.io/gorm"
)

type Role struct {
	ID        string `gorm:"type:uuid;primarykey"`
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	Permissions []*Permission `gorm:"many2many:role_permissions"`
}
