package gateways

import (
	socketio "github.com/googollee/go-socket.io"
	"go.uber.org/zap"
)

func NewTerminalSocketIOServer(
	terminalGateway *TerminalGateway,
	logger *zap.Logger,
) *socketio.Server {
	server := socketio.NewServer(nil)
	logger.Named("SocketIO")
	server.OnConnect("/", terminalGateway.OnConnect)
	server.OnEvent("/", "command", terminalGateway.RunCommand)
	server.OnDisconnect("/", terminalGateway.OnDisconnect)
	server.OnError("/", func(s socketio.Conn, e error) {
		logger.Error("Error in websocket connection", zap.Error(e))
	})

	return server
}
