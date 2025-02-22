package grpc

import (
	"context"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"os/signal"
	chatpb "project/api/grpc/pb/chat"
	chatpartpb "project/api/grpc/pb/chat_participant"
	messagepb "project/api/grpc/pb/message"
	userpb "project/api/grpc/pb/user"
	grpc2 "project/api/grpc/server"
	"project/services"
	"sync"
	"time"
)

type GRPCServer struct {
	server *grpc.Server
	port   string
}

func NewGRPCServer(port string) *GRPCServer {
	return &GRPCServer{
		server: grpc.NewServer(),
		port:   port,
	}
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

func (s *GRPCServer) Run(wg *sync.WaitGroup) {
	defer wg.Done()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		log.Println("Starting gRPS server on port: " + s.port)
		listener, err := net.Listen("tcp", ":"+s.port)
		if err != nil {
			log.Fatalf("Failed to listen: %v", err)
		}

		if err = s.server.Serve(listener); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	<-quit
	log.Println("Stopping gRPS server on port: " + s.port)

	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.server.GracefulStop()
	log.Println("Stopped gRPS server on port: " + s.port)
}
