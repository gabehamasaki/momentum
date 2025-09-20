package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        string `gorm:"type:uuid;primarykey"`
	Name      string
	Email     string
	RoleID    string
	Role      Role
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	permissions []*Permission `gorm:"many2many:user_permissions"`
}
