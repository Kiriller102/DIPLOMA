package grpc

import (
	"context"
	"database/sql"
	"errors"
	"github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	chatpb "project/services/Database_Service/api/grpc/pb/chat"
	chatpartpb "project/services/Database_Service/api/grpc/pb/chat_participant"
	messagepb "project/services/Database_Service/api/grpc/pb/message"
	userpb "project/services/Database_Service/api/grpc/pb/user"
	grpc2 "project/services/Database_Service/api/grpc/server"
	"project/services/Database_Service/services"
)

type GRPCServer struct{ server *grpc.Server }

func NewGRPCServer() *GRPCServer {
	server := grpc.NewServer(grpc.UnaryInterceptor(mapErrors))
	h := health.NewServer()
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, h)
	reflection.Register(server)
	return &GRPCServer{server: server}
}

func (s *GRPCServer) RegisterServices(userService *services.UserService,
	messageService *services.MessageService,
	chatService *services.ChatService,
	chatParticipantService *services.ChatParticipantService) {

	userGRPCServiec := grpc2.NewUserGRPCsService(userService)
	messageGRPCService := grpc2.NewMessageGRPCService(messageService)
	chatGRPCService := grpc2.NewChatGRPCService(chatService)
	chatParticipantGRPCService := grpc2.NewChatParticipantGRPCService(chatParticipantService)

	userpb.RegisterUserServiceServer(s.server, userGRPCServiec)
	messagepb.RegisterMessageServiceServer(s.server, messageGRPCService)
	chatpb.RegisterChatServiceServer(s.server, chatGRPCService)
	chatpartpb.RegisterChatParticipantServiceServer(s.server, chatParticipantGRPCService)
}

func (s *GRPCServer) Serve(listener net.Listener) error { return s.server.Serve(listener) }
func (s *GRPCServer) Shutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() { s.server.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		s.server.Stop()
		<-done
	}
}

func mapErrors(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	response, err := handler(ctx, req)
	if err == nil {
		return response, nil
	}
	if status.Code(err) != codes.Unknown {
		return nil, err
	}
	var pg *pq.Error
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, status.Error(codes.NotFound, "not found")
	case errors.Is(err, services.ErrInvalidInput):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.As(err, &pg):
		switch pg.Code {
		case "23505":
			return nil, status.Error(codes.AlreadyExists, "record already exists")
		case "23503", "22P02", "22001", "23502":
			return nil, status.Error(codes.InvalidArgument, "invalid fields or referenced record does not exist")
		}
	}
	return nil, status.Error(codes.Internal, "internal server error")
}
