package grpc

import (
	"context"
	"errors"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	userpb "project/api/grpc/pb/user"
	"project/models"
	"project/services"
)

type UserGRPCService struct {
	userpb.UnimplementedUserServiceServer
	userService *services.UserService
}

func NewUserGRPCsService(userService *services.UserService) *UserGRPCService {
	return &UserGRPCService{userService: userService}
}

func (s *UserGRPCService) CreateUser(ctx context.Context, req *userpb.CreateUserRequest) (*userpb.CreateUserResponse, error) {

	if req == nil || req.User == nil {
		log.Println("grpc error creating user: CreateUserRequest is nil")
		return &userpb.CreateUserResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: user is required",
			},
		}, errors.New("error: CreateUserRequest is nil")
	}

	user := models.User{
		Username:     req.User.Username,
		Email:        req.User.Email,
		FirstName:    req.User.FirstName,
		LastName:     req.User.LastName,
		PasswordHash: req.User.PasswordHash,
		IsActive:     req.User.IsActive,
	}

	err := s.userService.CreateUser(user)
	if err != nil {
		log.Printf("grpc error creating user: %v", err)
		return &userpb.CreateUserResponse{
			Error: &status.Status{
				Code:    int32(codes.Internal),
				Message: err.Error(),
			},
		}, err
	}

	return &userpb.CreateUserResponse{}, nil
}

func (s *UserGRPCService) GetUser(ctx context.Context, req *userpb.GetUserRequest) (*userpb.GetUserResponse, error) {

	if req == nil || req.Identifier == nil {
		log.Println("grpc error getting user: GetUserRequest is nil")
		return &userpb.GetUserResponse{
			Result: &userpb.GetUserResponse_Error{
				Error: &status.Status{
					Code:    int32(codes.InvalidArgument),
					Message: "invalid request: user is required",
				},
			},
		}, errors.New("error: GetUserRequest is nil")
	}

	var u models.User
	switch v := req.Identifier.(type) {
	case *userpb.GetUserRequest_Email:
		u.Email = v.Email
	case *userpb.GetUserRequest_Username:
		u.Username = v.Username
	case *userpb.GetUserRequest_Id:
		u.ID = int(v.Id)
	default:
		log.Println("grpc error getting user")
		return &userpb.GetUserResponse{
			Result: &userpb.GetUserResponse_Error{
				Error: &status.Status{
					Code:    int32(codes.InvalidArgument),
					Message: "invalid request",
				},
			},
		}, errors.New("grpc error getting user")
	}

	user, err := s.userService.GetUser(u)
	if err != nil {
		log.Printf("grpc error getting user: %v", err)
		return &userpb.GetUserResponse{
			Result: &userpb.GetUserResponse_Error{
				Error: &status.Status{
					Code:    int32(codes.Internal),
					Message: err.Error(),
				},
			},
		}, err
	}

	return &userpb.GetUserResponse{
		Result: &userpb.GetUserResponse_User{
			User: &userpb.User{
				Id:           int64(user.ID),
				Username:     user.Username,
				Email:        user.Email,
				FirstName:    user.FirstName,
				LastName:     user.LastName,
				PasswordHash: user.PasswordHash,
				CreatedAt:    timestamppb.New(user.CreatedAt),
				UpdatedAt:    timestamppb.New(user.UpdatedAt),
				IsActive:     user.IsActive,
			},
		},
	}, nil
}

func (s *UserGRPCService) GetAllUsers(ctx context.Context, _ *emptypb.Empty) (*userpb.GetAllUsersResponse, error) {
	users, err := s.userService.GetAllUsers()
	if err != nil {
		log.Printf("grpc error getting all users: %v", err)
		return &userpb.GetAllUsersResponse{
			Result: &userpb.GetAllUsersResponse_Error{
				Error: &status.Status{
					Code:    int32(codes.Internal),
					Message: err.Error(),
				},
			},
		}, err
	}

	var grpcUsers []*userpb.User
	for _, user := range users {
		grpcUsers = append(grpcUsers, &userpb.User{
			Id:           int64(user.ID),
			Username:     user.Username,
			Email:        user.Email,
			FirstName:    user.FirstName,
			LastName:     user.LastName,
			PasswordHash: user.PasswordHash,
			CreatedAt:    timestamppb.New(user.CreatedAt),
			UpdatedAt:    timestamppb.New(user.UpdatedAt),
			IsActive:     user.IsActive,
		})
	}

	return &userpb.GetAllUsersResponse{
		Result: &userpb.GetAllUsersResponse_Users{
			Users: &userpb.UserList{
				Users: grpcUsers,
			},
		},
	}, nil
}

func (s *UserGRPCService) UpdateUser(ctx context.Context, req *userpb.UpdateUserRequest) (*userpb.UpdateUserResponse, error) {
	if req == nil || req.User == nil {
		log.Println("grpc error updating user: UpdateUserRequest is nil")
		return &userpb.UpdateUserResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: user is required",
			},
		}, errors.New("invalid request: user is required")
	}

	user := models.User{
		ID:           int(req.User.Id),
		Username:     req.User.Username,
		Email:        req.User.Email,
		FirstName:    req.User.FirstName,
		LastName:     req.User.LastName,
		PasswordHash: req.User.PasswordHash,
		IsActive:     req.User.IsActive,
	}

	err := s.userService.UpdateUser(user)
	if err != nil {
		log.Printf("grpc error updating user: %v", err)
		return &userpb.UpdateUserResponse{
			Error: &status.Status{
				Code:    int32(codes.Internal),
				Message: err.Error(),
			},
		}, err
	}

	return &userpb.UpdateUserResponse{}, nil
}

func (s *UserGRPCService) DeleteUser(ctx context.Context, req *userpb.DeleteUserRequest) (*userpb.DeleteUserResponse, error) {

	if req == nil || req.Id == 0 {
		log.Println("grpc error deleting user: DeleteUserRequest is nil")
		return &userpb.DeleteUserResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: id is required",
			},
		}, errors.New("invalid request: id is required")
	}

	err := s.userService.DeleteUser(int(req.Id))
	if err != nil {
		log.Printf("grpc error deleting user: %v", err)
		return &userpb.DeleteUserResponse{
			Error: &status.Status{
				Code:    int32(codes.Internal),
				Message: err.Error(),
			},
		}, err
	}

	return &userpb.DeleteUserResponse{}, nil
}
