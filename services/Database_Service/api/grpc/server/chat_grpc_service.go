package grpc

import (
	"context"
	"errors"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	chatpb "project/api/grpc/pb/chat"
	"project/models"
	"project/services"
)

type ChatGRPCService struct {
	chatpb.UnimplementedChatServiceServer
	chatService *services.ChatService
}

func NewChatGRPCService(chatService *services.ChatService) *ChatGRPCService {
	return &ChatGRPCService{chatService: chatService}
}

func (s *ChatGRPCService) CreateChat(ctx context.Context, req *chatpb.CreateChatRequest) (*chatpb.CreateChatResponse, error) {

	if req == nil || req.Chat == nil {
		log.Println("error: CreateChatRequest is nil")
		return &chatpb.CreateChatResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: chat is required",
			},
		}, errors.New("invalid request: chat is required")
	}

	chat := models.Chat{
		Name:      req.Chat.Name,
		CreatedBy: int(req.Chat.CreatedBy),
	}

	if chat.Name == "" {
		return &chatpb.CreateChatResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "chat name is required",
			},
		}, errors.New("chat name is required")
	}

	if chat.CreatedBy == 0 {
		return &chatpb.CreateChatResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "chat created_by required",
			},
		}, errors.New("chat created_by required")
	}

	switch req.Chat.Type {
	case chatpb.ChatType_CHAT_TYPE_UNSPECIFIED:
		return &chatpb.CreateChatResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "chat type is required",
			},
		}, errors.New("chat type is required")
	case chatpb.ChatType_CHAT_TYPE_GROUP:
		chat.Type = models.ChatTypeGroup
	case chatpb.ChatType_CHAT_TYPE_PRIVATE:
		chat.Type = models.ChatTypePrivate
	}

	err := s.chatService.CreateChat(chat)
	if err != nil {
		log.Printf("error: create chat error: %v", err)
		return &chatpb.CreateChatResponse{
			Error: &status.Status{
				Code:    int32(codes.Internal),
				Message: err.Error(),
			},
		}, err
	}

	return &chatpb.CreateChatResponse{}, nil
}

func (s *ChatGRPCService) GetChat(ctx context.Context, req *chatpb.GetChatRequest) (*chatpb.GetChatResponse, error) {

	if req == nil || req.Id == 0 {
		log.Println("error: GetChatRequest is nil")
		return &chatpb.GetChatResponse{
			Result: &chatpb.GetChatResponse_Error{
				Error: &status.Status{
					Code:    int32(codes.InvalidArgument),
					Message: "invalid request: chat_id is required",
				},
			},
		}, errors.New("invalid request: chat_id is required")
	}

	chat, err := s.chatService.GetChatByID(int(req.Id))
	if err != nil {
		log.Printf("error: get chat by id error: %v", err)
		return &chatpb.GetChatResponse{
			Result: &chatpb.GetChatResponse_Error{
				Error: &status.Status{
					Code:    int32(codes.Internal),
					Message: err.Error(),
				},
			},
		}, err
	}

	return &chatpb.GetChatResponse{
		Result: &chatpb.GetChatResponse_Chat{
			Chat: &chatpb.Chat{
				Id:        int64(chat.ID),
				Name:      chat.Name,
				Type:      convertChatType(chat.Type),
				CreatedBy: int64(chat.CreatedBy),
				CreatedAt: timestamppb.New(chat.CreatedAt),
			},
		},
	}, nil
}

func (s *ChatGRPCService) UpdateChat(ctx context.Context, req *chatpb.UpdateChatRequest) (*chatpb.UpdateChatResponse, error) {
	if req == nil || req.Chat == nil {
		log.Println("error: CreateChatRequest is nil")
		return &chatpb.UpdateChatResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: chat is required",
			},
		}, errors.New("invalid request: chat is required")
	}

	if req.Chat.Name == "" {
		log.Println("error: chat name is nil")
		return &chatpb.UpdateChatResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "Chat name is required",
			},
		}, errors.New("chat name is required")
	}

	if req.Chat.Id == 0 {
		log.Println("error: chat id is nil")
		return &chatpb.UpdateChatResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "Chat id is required",
			},
		}, errors.New("chat id is required")
	}

	chat := models.Chat{
		ID:   int(req.Chat.Id),
		Name: req.Chat.Name,
	}
	err := s.chatService.UpdateChat(chat)
	if err != nil {
		log.Printf("error: update chat error: %v", err)
		return &chatpb.UpdateChatResponse{
			Error: &status.Status{
				Code:    int32(codes.Internal),
				Message: err.Error(),
			},
		}, err
	}

	return &chatpb.UpdateChatResponse{}, nil
}

func (s *ChatGRPCService) DeleteChat(ctx context.Context, req *chatpb.DeleteChatRequest) (*chatpb.DeleteChatResponse, error) {
	if req == nil || req.Id == 0 {
		log.Println("error: DeleteChatRequest is nil")
		return &chatpb.DeleteChatResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: id is required",
			},
		}, errors.New("invalid request: id is required")
	}

	err := s.chatService.DeleteChatByID(int(req.Id))
	if err != nil {
		log.Printf("error: delete chat by id error: %v", err)
		return &chatpb.DeleteChatResponse{
			Error: &status.Status{
				Code:    int32(codes.Internal),
				Message: err.Error(),
			},
		}, err
	}

	return &chatpb.DeleteChatResponse{}, nil
}

func convertChatType(t models.ChatType) chatpb.ChatType {
	switch t {
	case models.ChatTypeGroup:
		return chatpb.ChatType_CHAT_TYPE_GROUP
	case models.ChatTypePrivate:
		return chatpb.ChatType_CHAT_TYPE_PRIVATE
	default:
		return chatpb.ChatType_CHAT_TYPE_UNSPECIFIED
	}
}
