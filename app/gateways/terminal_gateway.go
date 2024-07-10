package gateways

import (
	"ai-developer/app/middleware"
	"github.com/creack/pty"
	socketio "github.com/googollee/go-socket.io"
	"go.uber.org/zap"
	"os/exec"
)

type TerminalGateway struct {
	logger  *zap.Logger
	jwtAuth *middleware.JWTClaims
}

func (w *TerminalGateway) OnConnect(s socketio.Conn) error {
	s.SetContext(make(map[string]interface{}))
	w.logger.Info("Terminal websocket connection established", zap.String("connection_id", s.ID()))
	return nil
}

func (w *TerminalGateway) OnDisconnect(s socketio.Conn, reason string) {
	s.SetContext(make(map[string]interface{}))
	w.logger.Info("Terminal websocket disconnection", zap.String("connection_id", s.ID()), zap.String("reason", reason))
}

func (w *TerminalGateway) RunCommand(s socketio.Conn, data map[string]interface{}) {
	w.logger.Info("Terminal websocket command received", zap.String("connection_id", s.ID()), zap.Any("data", data))
	// Execute the command
	c := exec.Command("bash")
	f, err := pty.Start(c)
	if err != nil {
		panic(err)
	}
	command, ok := data["command"].(string)
	if !ok {
		w.logger.Error("Invalid command type")
		s.Emit("error", "Invalid command type")
		return
	}
	commandString := command + "\n"

	go func() {
		f.Write([]byte(commandString))
		f.Write([]byte{4}) // EOT
	}()
	//io.Copy(os.Stdout, f)
	// instead of printing to stdout, we can emit the output to the client
	go func() {
		buf := make([]byte, 100)
		for {
			n, err := f.Read(buf)
			if err != nil {
				w.logger.Error("Error reading from terminal", zap.Error(err))
				return
			}
			s.Emit("output", string(buf[:n]))
		}
	}()
}

func NewTerminalGateway(
	jwtAuth *middleware.JWTClaims,
	logger *zap.Logger,
) *TerminalGateway {
	return &TerminalGateway{
		jwtAuth: jwtAuth,
		logger:  logger.Named("TerminalGateway"),
	}
}
