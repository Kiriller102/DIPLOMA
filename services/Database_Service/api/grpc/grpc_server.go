package grpc

import (
	"context"
	"database/sql"
	"errors"
	"github.com/lib/pq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"log"
	"log/slog"
	"net"
	"os"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/internal/logging"
	"strings"

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

func NewGRPCServer(access *auth.Service, loggers ...*slog.Logger) *GRPCServer {
	logger := logging.New(os.Stderr, nil)
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(historyCompletion(logger), mapErrors, authenticate(access)), grpc.StreamInterceptor(authenticateStream(access)))
	h := health.NewServer()
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, h)
	reflection.Register(server)
	return &GRPCServer{server: server}
}

func historyCompletion(logger *slog.Logger) grpc.UnaryServerInterceptor {
	classifier := logging.NewClassifier(logging.Sentinels{InvalidInput: services.ErrInvalidInput, Unauthenticated: auth.ErrUnauthenticated, Forbidden: auth.ErrForbidden})
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (response interface{}, err error) {
		if info.FullMethod != messagepb.MessageService_GetMessagesByChatID_FullMethodName {
			return next(ctx, req)
		}
		ctx, completion := logging.Begin(ctx, logger, classifier, "grpc", logging.MessageHistory)
		returned := false
		defer func() {
			if !returned {
				logging.Capture(ctx, errors.New("RPC interrupted"), "handler")
			}
			code := status.Code(err)
			if !returned {
				code = codes.Unknown
			}
			completion.Finish(ctx, slog.String("rpc_method", info.FullMethod), slog.String("rpc_code", code.String()))
		}()
		response, err = next(ctx, req)
		logging.Capture(ctx, err, "rpc")
		returned = true
		return response, err
	}
}

func (s *GRPCServer) RegisterServices(userService *services.UserService,
	messageService *services.MessageService,
	chatService *services.ChatService,
	chatParticipantService *services.ChatParticipantService, access *auth.Service) {

	userGRPCServiec := grpc2.NewUserGRPCsService(userService, access)
	messageGRPCService := grpc2.NewMessageGRPCService(messageService, access)
	chatGRPCService := grpc2.NewChatGRPCService(chatService, access)
	chatParticipantGRPCService := grpc2.NewChatParticipantGRPCService(chatParticipantService, access)

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
	logging.Capture(ctx, err, "rpc")
	if status.Code(err) != codes.Unknown {
		return nil, err
	}
	var pg *pq.Error
	switch {
	case errors.Is(err, auth.ErrUnauthenticated):
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	case errors.Is(err, auth.ErrForbidden):
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, sql.ErrNoRows):
		return nil, status.Error(codes.NotFound, "not found")
	case errors.Is(err, services.ErrInvalidInput):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), ctx.Err() != nil:
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		return nil, status.FromContextError(err).Err()
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

func authenticatedContext(ctx context.Context, method string, access *auth.Service) (context.Context, error) {
	if strings.HasPrefix(method, "/grpc.health.v1.Health/") {
		return ctx, nil
	}
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("authorization")
	if len(values) != 1 {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	token, err := auth.Bearer(values[0])
	if err == nil {
		var next context.Context
		next, err = access.Authenticate(ctx, token)
		if err == nil {
			return next, nil
		}
	}
	logging.Capture(ctx, err, "authentication")
	if errors.Is(err, auth.ErrUnauthenticated) {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	if errors.Is(err, auth.ErrForbidden) {
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	}
	if ctx.Err() != nil {
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, status.FromContextError(err).Err()
	}
	log.Printf("authenticate gRPC request: %v", err)
	return nil, status.Error(codes.Unavailable, "authentication temporarily unavailable")
}

func authenticate(access *auth.Service) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		next, err := authenticatedContext(ctx, info.FullMethod, access)
		if err != nil {
			logging.Capture(ctx, err, "authentication")
			return nil, err
		}
		return handler(next, req)
	}
}

type authenticatedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authenticatedStream) Context() context.Context { return s.ctx }

func authenticateStream(access *auth.Service) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, err := authenticatedContext(stream.Context(), info.FullMethod, access)
		if err != nil {
			return err
		}
		return handler(srv, &authenticatedStream{ServerStream: stream, ctx: ctx})
	}
}
