package grpc

import (
	"context"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	messagepb "project/services/Database_Service/api/grpc/pb/message"
	"project/services/Database_Service/models"
	"project/services/Database_Service/services"
)

type MessageGRPCService struct {
	messagepb.UnimplementedMessageServiceServer
	messageService *services.MessageService
}

func NewMessageGRPCService(messageService *services.MessageService) *MessageGRPCService {
	return &MessageGRPCService{messageService: messageService}
}

func (s *MessageGRPCService) CreateMessage(ctx context.Context, req *messagepb.CreateMessageRequest) (*messagepb.CreateMessageResponse, error) {
	if req == nil || req.Message == nil {
		log.Println("grpc error creating message: CreatingMessageRequest is nil")
		return &messagepb.CreateMessageResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: message required",
			},
		}, grpcstatus.Error(codes.InvalidArgument, "invalid request: CreateMessageRequest is nil")
	}

	message := models.Message{
		ChatID:   int(req.Message.ChatId),
		SenderID: int(req.Message.SenderId),
		Content:  req.Message.Context,
	}

	switch req.Message.EncryptionAlgorithm {
	case messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_UNSPECIFIED:
		message.EncryptionAlgorithm = ""
	case messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_AES:
		message.EncryptionAlgorithm = models.EncryptionAlgorithmAES
	case messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_KUZNECHIK:
		message.EncryptionAlgorithm = models.EncryptionAlgorithmKuznechik
	default:
		return &messagepb.CreateMessageResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: EncryptionAlgorithmType is required",
			},
		}, grpcstatus.Error(codes.InvalidArgument, "invalid request: EncryptionAlgorithmType is required")
	}

	err := s.messageService.CreateMessage(&message)
	if err != nil {
		log.Println("grpc error creating message: ", err)
		return &messagepb.CreateMessageResponse{
			Error: &status.Status{
				Code:    int32(codes.Internal),
				Message: err.Error(),
			},
		}, err
	}

	return &messagepb.CreateMessageResponse{}, nil
}

func (s *MessageGRPCService) GetMessage(ctx context.Context, req *messagepb.GetMessageRequest) (*messagepb.GetMessageResponse, error) {
	if req == nil || req.Id == 0 {
		log.Println("grpc error getting message: GetMessageRequest is nil")
		return &messagepb.GetMessageResponse{
			Result: &messagepb.GetMessageResponse_Error{
				Error: &status.Status{
					Code:    int32(codes.InvalidArgument),
					Message: "invalid request: id is required",
				},
			},
		}, grpcstatus.Error(codes.InvalidArgument, "invalid request: id is required")
	}

	message, err := s.messageService.GetMessageByID(int(req.Id))
	if err != nil {
		log.Println("grpc error getting message: ", err)
		return &messagepb.GetMessageResponse{
			Result: &messagepb.GetMessageResponse_Error{
				Error: &status.Status{
					Code:    int32(codes.Internal),
					Message: err.Error(),
				},
			},
		}, err
	}

	grpcMessage := messagepb.Message{
		Id:       int64(message.ID),
		ChatId:   int64(message.ChatID),
		SenderId: int64(message.SenderID),
		Context:  message.Content,
		SentAd:   timestamppb.New(message.SentAt),
	}
	switch message.EncryptionAlgorithm {
	case models.EncryptionAlgorithmAES:
		grpcMessage.EncryptionAlgorithm = messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_AES
	case models.EncryptionAlgorithmKuznechik:
		grpcMessage.EncryptionAlgorithm = messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_KUZNECHIK
	default:
		grpcMessage.EncryptionAlgorithm = messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_UNSPECIFIED
	}

	return &messagepb.GetMessageResponse{
		Result: &messagepb.GetMessageResponse_Message{
			Message: &grpcMessage,
		},
	}, nil
}

func (s *MessageGRPCService) UpdateMessage(ctx context.Context, req *messagepb.UpdateMessageRequest) (*messagepb.UpdateMessageResponse, error) {
	if req == nil || req.Message == nil {
		log.Println("grpc error updating message: UpdateMessageRequest is nil")
		return &messagepb.UpdateMessageResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid request: message is required",
			},
		}, grpcstatus.Error(codes.InvalidArgument, "invalid request: message is required")
	}

	message := models.Message{
		ID:      int(req.Message.Id),
		Content: req.Message.Context,
	}

	switch req.Message.EncryptionAlgorithm {
	case messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_UNSPECIFIED:
		message.EncryptionAlgorithm = ""
	case messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_AES:
		message.EncryptionAlgorithm = models.EncryptionAlgorithmAES
	case messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_KUZNECHIK:
		message.EncryptionAlgorithm = models.EncryptionAlgorithmKuznechik
	default:
		return &messagepb.UpdateMessageResponse{
			Error: &status.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "error: incorrect EncryptionAlgorithmType",
			},
		}, grpcstatus.Error(codes.InvalidArgument, "invalid request: EncryptionAlgorithmType is required")
	}

	err := s.messageService.UpdateMessage(message)
	if err != nil {
		log.Println("grpc error updating message: ", err)
		return &messagepb.UpdateMessageResponse{
			Error: &status.Status{
				Code:    int32(codes.Internal),
				Message: err.Error(),
			},
		}, err
	}

	return &messagepb.UpdateMessageResponse{}, nil
}

func (s *MessageGRPCService) GetMessagesByChatID(ctx context.Context, req *messagepb.GetMessagesByChatIDRequest) (*messagepb.GetMessagesByChatIDResponse, error) {
	if req == nil || req.ChatId == 0 {
		log.Println("grpc error getting message: GetMessagesByChatIDRequest is nil")
		return &messagepb.GetMessagesByChatIDResponse{
			Result: &messagepb.GetMessagesByChatIDResponse_Error{
				Error: &status.Status{
					Code:    int32(codes.InvalidArgument),
					Message: "invalid request: chat_id is required",
				},
			},
		}, grpcstatus.Error(codes.InvalidArgument, "invalid request: chat_id is required")
	}

	messages, err := s.messageService.GetMessagesByChatID(int(req.ChatId))
	if err != nil {
		log.Println("grpc error getting message: ", err)
		return &messagepb.GetMessagesByChatIDResponse{
			Result: &messagepb.GetMessagesByChatIDResponse_Error{
				Error: &status.Status{
					Code:    int32(codes.Internal),
					Message: err.Error(),
				},
			},
		}, err
	}

	var grpcMessages []*messagepb.Message
	for _, message := range messages {
		grpcMessages = append(grpcMessages, &messagepb.Message{
			Id:                  int64(message.ID),
			ChatId:              int64(message.ChatID),
			SenderId:            int64(message.SenderID),
			Context:             message.Content,
			EncryptionAlgorithm: convertEncryptionType(message.EncryptionAlgorithm),
			SentAd:              timestamppb.New(message.SentAt),
		})
	}

	return &messagepb.GetMessagesByChatIDResponse{
		Result: &messagepb.GetMessagesByChatIDResponse_Messages{
			Messages: &messagepb.MessageList{
				Message: grpcMessages,
			},
		},
	}, nil
}

func convertEncryptionType(t models.EncryptionAlgorithmType) messagepb.EncryptionAlgorithmType {
	switch t {
	case models.EncryptionAlgorithmAES:
		return messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_AES
	case models.EncryptionAlgorithmKuznechik:
		return messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_KUZNECHIK
	default:
		return messagepb.EncryptionAlgorithmType_ENCRYPTION_ALGORITHM_UNSPECIFIED
	}
}
