package grpc

import (
	"context"
	"errors"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	chatpartpb "project/api/grpc/pb/chat_participant"
	"project/models"
	"project/services"
)

type ChatParticipantGRPCService struct {
	chatpartpb.UnimplementedChatParticipantServiceServer
	chatPartService *services.ChatParticipantService
}

func NewChatParticipantGRPCService(chatPartService *services.ChatParticipantService) *ChatParticipantGRPCService {
	return &ChatParticipantGRPCService{chatPartService: chatPartService}
}

func (s *ChatParticipantGRPCService) CreateChatParticipant(ctx context.Context, req *chatpartpb.CreateChatParticipantRequest) (*chatpartpb.CreateChatParticipantResponse, error) {
	if req == nil || req.ChatParticipant == nil {
		log.Println("error: CreteChatParticipantRequest is nil")
		return &chatpartpb.CreateChatParticipantResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: chat participant is required",
			},
		}, errors.New("invalid request: chat participant is required")
	}

	chatParticipant := models.ChatParticipant{
		ChatID: int(req.ChatParticipant.ChatId),
		UserID: int(req.ChatParticipant.UserId),
	}
	switch req.ChatParticipant.UserRole {
	case chatpartpb.UserRoleType_USER_ROLE_ADMIN:
		chatParticipant.UserRole = models.UserRoleAdmin
	case chatpartpb.UserRoleType_USER_ROLE_MEMBER:
		chatParticipant.UserRole = models.UserRoleMember
	default:
		return &chatpartpb.CreateChatParticipantResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: chat participant role is required",
			},
		}, errors.New("invalid request: chat participant role is required")
	}

	err := s.chatPartService.CreateChatParticipant(chatParticipant)
	if err != nil {
		return &chatpartpb.CreateChatParticipantResponse{
			Error: &status.Status{
				Code:    int32(codes.Internal),
				Message: err.Error(),
			},
		}, err
	}

	return &chatpartpb.CreateChatParticipantResponse{}, nil
}

func (s *ChatParticipantGRPCService) DeleteChatParticipant(ctx context.Context, req *chatpartpb.DeleteChatParticipantRequest) (*chatpartpb.DeleteChatParticipantResponse, error) {
	if req == nil || req.ChatParticipant == nil {
		log.Println("error: DeleteChatParticipantRequest is nil")
		return &chatpartpb.DeleteChatParticipantResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: chat participant is nil",
			},
		}, errors.New("invalid request: chat participant is required")
	}

	cp := models.ChatParticipant{
		ChatID: int(req.ChatParticipant.ChatId),
		UserID: int(req.ChatParticipant.UserId),
	}

	err := s.chatPartService.DeleteParticipantByUserID(cp)
	if err != nil {
		return &chatpartpb.DeleteChatParticipantResponse{
			Error: &status.Status{
				Code:    int32(codes.Internal),
				Message: err.Error(),
			},
		}, err
	}

	return &chatpartpb.DeleteChatParticipantResponse{}, nil
}

func (s *ChatParticipantGRPCService) DeleteAllChatParticipants(ctx context.Context, req *chatpartpb.DeleteAllChatParticipantsRequest) (*chatpartpb.DeleteAllChatParticipantsResponse, error) {
	if req == nil || req.ChatId == 0 {
		log.Println("error: DeleteAllChatParticipantRequest is nil")
		return &chatpartpb.DeleteAllChatParticipantsResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: chat participant is required",
			},
		}, errors.New("invalid request: chat participant is required")
	}

	err := s.chatPartService.DeleteAllParticipantsByChatID(int(req.ChatId))
	if err != nil {
		log.Printf("error: DeleteAllChatParticipantsByChatID is invalid: %v", err.Error())
		return &chatpartpb.DeleteAllChatParticipantsResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: err.Error(),
			},
		}, err
	}

	return &chatpartpb.DeleteAllChatParticipantsResponse{}, nil
}

func (s *ChatParticipantGRPCService) GetChatParticipants(ctx context.Context, req *chatpartpb.GetChatParticipantsByChatIDRequest) (*chatpartpb.GetChatParticipantsByChatIDResponse, error) {
	if req == nil || req.ChatId == 0 {
		log.Println("error: GetChatParticipantsRequest is nil")
		return &chatpartpb.GetChatParticipantsByChatIDResponse{
			Result: &chatpartpb.GetChatParticipantsByChatIDResponse_Error{
				Error: &status.Status{
					Code:    int32(codes.InvalidArgument),
					Message: "invalid request: chat_id is required",
				},
			},
		}, errors.New("invalid request: chat_id is required")
	}

	chatParticipants, err := s.chatPartService.GetAllChatParticipants(int(req.ChatId))
	if err != nil {
		log.Printf("error: GetChatParticipants is invalid: %v", err.Error())
		return &chatpartpb.GetChatParticipantsByChatIDResponse{
			Result: &chatpartpb.GetChatParticipantsByChatIDResponse_Error{
				Error: &status.Status{
					Code:    int32(codes.Internal),
					Message: err.Error(),
				},
			},
		}, err
	}

	var chatParticipantsGRPC []*chatpartpb.ChatParticipant

	for _, chatParticipant := range chatParticipants {
		chatParticipantsGRPC = append(chatParticipantsGRPC, &chatpartpb.ChatParticipant{
			ChatId:   int64(chatParticipant.ChatID),
			UserId:   int64(chatParticipant.UserID),
			UserRole: convertUserRoleType(chatParticipant.UserRole),
			JoinedAt: timestamppb.New(chatParticipant.JoinedAt),
		})
	}

	return &chatpartpb.GetChatParticipantsByChatIDResponse{
		Result: &chatpartpb.GetChatParticipantsByChatIDResponse_ChatParticipants{
			ChatParticipants: &chatpartpb.ChatParticipantList{
				ChatParticipants: chatParticipantsGRPC,
			},
		},
	}, nil
}

func convertUserRoleType(role models.UserRoleType) chatpartpb.UserRoleType {
	switch role {
	case models.UserRoleMember:
		return chatpartpb.UserRoleType_USER_ROLE_MEMBER
	case models.UserRoleAdmin:
		return chatpartpb.UserRoleType_USER_ROLE_ADMIN
	default:
		return chatpartpb.UserRoleType_USER_ROLE_UNSPECIFIED
	}
}
