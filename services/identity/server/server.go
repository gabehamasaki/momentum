package server

import (
	"context"

	"github.com/gabehamasaki/momentum/services/identity/models"
	"github.com/gabehamasaki/momentum/services/identity/services"
	"github.com/gabehamasaki/momentum/services/identity/utils"
	"github.com/gabehamasaki/momentum/shared/v1/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"go.uber.org/zap"
)

type IdentityServer struct {
	proto.UnimplementedIdentityServiceServer
	logger      *zap.Logger
	userService *services.UserService
}

func NewIdentityServer(userService *services.UserService, logger *zap.Logger) *IdentityServer {
	return &IdentityServer{userService: userService}
}

func (s *IdentityServer) GetUsers(ctx context.Context, empty *empty.Empty) (*proto.UsersResponse, error) {
	users, err := s.userService.GetUsers(ctx)
	if err != nil {
		return nil, err
	}

	var protoUsers []*proto.User
	for _, user := range users {
		protoUser := &proto.User{
			Id:        user.ID,
			Name:      user.Name,
			Email:     user.Email,
			Role:      user.Role.Name,
			CreatedAt: user.CreatedAt.Unix(),
		}
		protoUsers = append(protoUsers, protoUser)
	}

	return &proto.UsersResponse{Users: protoUsers}, nil
}

func (s *IdentityServer) GetUser(ctx context.Context, req *proto.UserRequest) (*proto.UserResponse, error) {
	user, err := s.userService.FindUserByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	protoUser := &proto.User{
		Id:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		Role:      user.Role.Name,
		CreatedAt: user.CreatedAt.Unix(),
	}

	var protoPermissions []*proto.Permission
	for _, perm := range user.Permissions {
		protoPerm := &proto.Permission{
			Id:   int64(perm.ID),
			Name: perm.Name,
		}
		protoPermissions = append(protoPermissions, protoPerm)
	}

	var protoRolePermissions []*proto.Permission
	for _, perm := range user.Role.Permissions {
		protoPerm := &proto.Permission{
			Id:   int64(perm.ID),
			Name: perm.Name,
		}
		protoRolePermissions = append(protoRolePermissions, protoPerm)
	}

	return &proto.UserResponse{
		User: protoUser,
		Role: &proto.Role{
			Id:          user.Role.ID,
			Name:        user.Role.Name,
			Permissions: protoRolePermissions,
		},
		Permissions: protoPermissions,
	}, nil
}

func (s *IdentityServer) StoreUser(ctx context.Context, req *proto.StoreUserRequest) (*proto.StoreUserResponse, error) {
	hashedPassword, err := utils.Bcrypt(req.GetPassword())
	if err != nil {
		return nil, err
	}

	userToStore := models.User{
		Name:     req.GetName(),
		Email:    req.GetEmail(),
		Password: hashedPassword,
		RoleID:   req.GetRoleId(),
	}

	storedUser, err := s.userService.StoreUser(ctx, userToStore)
	if err != nil {
		return nil, err
	}

	return &proto.StoreUserResponse{
		User: &proto.User{
			Id:    storedUser.ID,
			Name:  storedUser.Name,
			Email: storedUser.Email,
		},
	}, nil
}
