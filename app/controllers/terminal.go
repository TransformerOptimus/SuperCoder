package controllers

import (
	"ai-developer/app/types/request"
	"ai-developer/app/utils"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/creack/pty"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type TTYSize struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
	X    uint16 `json:"x"`
	Y    uint16 `json:"y"`
}

var WebsocketMessageType = map[int]string{
	websocket.BinaryMessage: "binary",
	websocket.TextMessage:   "text",
	websocket.CloseMessage:  "close",
	websocket.PingMessage:   "ping",
	websocket.PongMessage:   "pong",
}

type TerminalController struct {
	DefaultConnectionErrorLimit int
	MaxBufferSizeBytes          int
	KeepalivePingTimeout        time.Duration
	ConnectionErrorLimit        int
	cmd                         *exec.Cmd
	Command                     string
	Arguments                   []string
	AllowedHostnames            []string
	logger                      *zap.Logger
	tty                         *os.File
	cancelFunc                  context.CancelFunc
}

func NewTerminalController(logger *zap.Logger, command string, arguments []string, allowedHostnames []string) (*TerminalController, error) {
	cmd := exec.Command(command, arguments...)
	tty, err := pty.Start(cmd)
	if err != nil {
		logger.Warn("failed to start command", zap.Error(err))
		return nil, err
	}
	return &TerminalController{
		DefaultConnectionErrorLimit: 10,
		MaxBufferSizeBytes:          1024,
		KeepalivePingTimeout:        20 * time.Second,
		ConnectionErrorLimit:        10,
		tty:                         tty,
		cmd:                         cmd,
		Arguments:                   arguments,
		AllowedHostnames:            allowedHostnames,
		logger:                      logger,
	}, nil
}

func (controller *TerminalController) RunCommand(ctx *gin.Context) {
	var commandRequest request.RunCommandRequest
	if err := ctx.ShouldBindJSON(&commandRequest); err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}
	command := commandRequest.Command
	if command == "" {
		ctx.JSON(400, gin.H{"error": "command is required"})
		return
	}
	if !strings.HasSuffix(command, "\n") {
		command += "\n"
	}

	_, err := controller.tty.Write([]byte(command))
	if err != nil {
		return
	}
}

func (controller *TerminalController) NewTerminal(ctx *gin.Context) {
	subCtx, cancelFunc := context.WithCancel(ctx)
	controller.cancelFunc = cancelFunc

	defer func() {
		controller.cleanup()
	}()

	connection, err := controller.setupConnection(ctx, ctx.Writer, ctx.Request)
	if err != nil {
		controller.logger.Warn("failed to setup connection", zap.Error(err))
		return
	}

	var waiter sync.WaitGroup

	waiter.Add(3)

	go controller.keepAlive(subCtx, connection, &waiter)

	go controller.readFromTTY(subCtx, connection, &waiter)

	go controller.writeToTTY(subCtx, connection, &waiter)

	waiter.Wait()

	controller.logger.Info("closing connection...")
}

func (controller *TerminalController) setupConnection(ctx context.Context, w gin.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	upgrader := utils.GetConnectionUpgrader(controller.AllowedHostnames, controller.MaxBufferSizeBytes)
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	return connection, nil
}

func (controller *TerminalController) keepAlive(ctx context.Context, connection *websocket.Conn, waiter *sync.WaitGroup) {
	defer waiter.Done()
	lastPongTime := time.Now()
	keepalivePingTimeout := controller.KeepalivePingTimeout

	connection.SetPongHandler(func(msg string) error {
		lastPongTime = time.Now()
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := connection.WriteMessage(websocket.PingMessage, []byte("keepalive")); err != nil {
				controller.logger.Warn("failed to write ping message", zap.Error(err))
				return
			}

			time.Sleep(keepalivePingTimeout / 2)

			if time.Now().Sub(lastPongTime) > keepalivePingTimeout {
				controller.logger.Warn("failed to get response from ping, triggering disconnect now...")
				return
			}
			controller.logger.Debug("received response from ping successfully")
		}
	}
}

func (controller *TerminalController) readFromTTY(ctx context.Context, connection *websocket.Conn, waiter *sync.WaitGroup) {
	defer waiter.Done()
	errorCounter := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		buffer := make([]byte, controller.MaxBufferSizeBytes)

		readLength, err := controller.tty.Read(buffer)
		if err != nil {
			controller.logger.Warn("failed to read from tty", zap.Error(err))
			if err := connection.WriteMessage(websocket.TextMessage, []byte("bye!")); err != nil {
				controller.logger.Warn("failed to send termination message from tty to xterm.js", zap.Error(err))
			}
			return
		}

		if err := connection.WriteMessage(websocket.BinaryMessage, buffer[:readLength]); err != nil {
			controller.logger.Warn(fmt.Sprintf("failed to send %v bytes from tty to xterm.js", readLength), zap.Int("read_length", readLength), zap.Error(err))
			errorCounter++
			if errorCounter > controller.ConnectionErrorLimit {
				return
			}
			continue
		}
		controller.logger.Info(fmt.Sprintf("sent message of size %v bytes from tty to xterm.js", readLength), zap.Int("read_length", readLength))
		errorCounter = 0
	}
}

func (controller *TerminalController) writeToTTY(ctx context.Context, connection *websocket.Conn, waiter *sync.WaitGroup) {
	defer waiter.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		messageType, data, err := connection.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				controller.logger.Info("WebSocket closed by client")
				break
			}
			controller.logger.Warn("failed to get next reader", zap.Error(err))
			return
		}

		dataLength := len(data)
		dataBuffer := bytes.Trim(data, "\x00")
		dataType := WebsocketMessageType[messageType]

		controller.logger.Info(fmt.Sprintf("received %s (type: %v) message of size %v byte(s) from xterm.js with key sequence: %v", dataType, messageType, dataLength, dataBuffer))

		if messageType == websocket.BinaryMessage && dataBuffer[0] == 1 {
			controller.resizeTTY(dataBuffer)
			continue
		}

		bytesWritten, err := controller.tty.Write(dataBuffer)
		if err != nil {
			controller.logger.Warn(fmt.Sprintf("failed to write %v bytes to tty: %s", len(dataBuffer), err))
			continue
		}
		controller.logger.Info("bytes written to tty...", zap.Int("bytes_written", bytesWritten))
	}
}

func (controller *TerminalController) resizeTTY(dataBuffer []byte) {
	ttySize := &TTYSize{}
	resizeMessage := bytes.Trim(dataBuffer[1:], " \n\r\t\x00\x01")
	if err := json.Unmarshal(resizeMessage, ttySize); err != nil {
		controller.logger.Warn(fmt.Sprintf("failed to unmarshal received resize message '%s'", resizeMessage), zap.ByteString("resizeMessage", resizeMessage), zap.Error(err))
		return
	}
	controller.logger.Info("resizing tty ", zap.Uint16("rows", ttySize.Rows), zap.Uint16("cols", ttySize.Cols))
	if err := pty.Setsize(controller.tty, &pty.Winsize{
		Rows: ttySize.Rows,
		Cols: ttySize.Cols,
	}); err != nil {
		controller.logger.Warn("failed to resize tty", zap.Error(err))
	}
}

func (controller *TerminalController) cleanup() {
	controller.logger.Info("gracefully stopping spawned tty...")
	if controller.cmd.Process != nil {
		if err := controller.cmd.Process.Kill(); err != nil {
			controller.logger.Warn("failed to kill process: %s", zap.Error(err))
		}
		if _, err := controller.cmd.Process.Wait(); err != nil {
			controller.logger.Warn("failed to wait for process to exit: %s", zap.Error(err))
		}
	}
	if controller.cancelFunc != nil {
		controller.cancelFunc()
	}
}
