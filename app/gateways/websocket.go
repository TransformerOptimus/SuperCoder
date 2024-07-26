package gateways

import (
	socketio "github.com/googollee/go-socket.io"
	"go.uber.org/zap"
)

type SocketIOServer struct {
	Server *socketio.Server
	Logger *zap.Logger
}

func NewSocketIOServer(
	workspaceGateway *WorkspaceGateway,
	logger *zap.Logger,
) *SocketIOServer {
	server := socketio.NewServer(nil)
	logger.Named("SocketIO")
	server.OnConnect("/", workspaceGateway.OnConnect)
	server.OnEvent("/", "workspace-start", workspaceGateway.OnWorkspaceStartEvent)
	server.OnEvent("/", "workspace-close", workspaceGateway.OnWorkspaceDeleteEvent)
	server.OnDisconnect("/", workspaceGateway.OnDisconnect)
	server.OnError("/", func(s socketio.Conn, e error) {
		logger.Error("Error in websocket connection", zap.Error(e))
	})

	return &SocketIOServer{
		Server: server,
		Logger: logger,
	}
}

// BroadcastToRoom sends a message to a specific room
func (s *SocketIOServer) BroadcastToRoom(room, event, message string) {
	messageSent := s.Server.BroadcastToRoom("/", room, event, message)
	if !messageSent {
		s.Logger.Error("Failed to broadcast message", zap.String("room", room), zap.String("event", event))
	}
}
