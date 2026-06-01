package gateways

import (
	socketio "github.com/googollee/go-socket.io"
	"go.uber.org/zap"
)

func NewSocketIOServer(
	workspaceGateway *WorkspaceGateway,
	logger *zap.Logger,
) *socketio.Server {
	server := socketio.NewServer(nil)
	logger.Named("SocketIO")
	server.OnConnect("/", workspaceGateway.OnConnect)
	server.OnEvent("/", "workspace-start", workspaceGateway.OnWorkspaceStartEvent)
	server.OnEvent("/", "workspace-close", workspaceGateway.OnWorkspaceDeleteEvent)
	server.OnDisconnect("/", workspaceGateway.OnDisconnect)
	server.OnError("/", func(s socketio.Conn, e error) {
		logger.Error("Error in websocket connection", zap.Error(e))
	})

	return server
}
