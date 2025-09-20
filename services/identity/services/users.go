package services

import (
	"context"
	"fmt"

	"github.com/gabehamasaki/momentum/services/identity/database"
	"github.com/gabehamasaki/momentum/services/identity/models"
)

type UserService struct {
	db *database.Database
}

func NewUserService(db *database.Database) *UserService {
	return &UserService{db: db}
}

func (s *UserService) GetUsers(ctx context.Context) ([]models.User, error) {
	var users []models.User
	conn, err := s.db.ConnWithContext(ctx)
	if err != nil {
		return nil, err
	}
	defer s.db.Close()

	if err := conn.Preload("Role", nil).Find(&users).Error; err != nil {
		return nil, err
	}

	return users, nil
}

func (s *UserService) StoreUser(ctx context.Context, user models.User) (models.User, error) {
	conn, err := s.db.ConnWithContext(ctx)
	if err != nil {
		return models.User{}, err
	}
	defer s.db.Close()

	if err := conn.Create(&user).Error; err != nil {
		fmt.Println("Erro ao criar usuário:", err)
		return models.User{}, err
	}

	return user, nil
}
