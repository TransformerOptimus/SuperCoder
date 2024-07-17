package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"ai-developer/app/types/request"
	"ai-developer/app/utils"

	"github.com/creack/pty"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
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
	writeMutex                  sync.Mutex
	historyBuffer               bytes.Buffer
}

func NewTerminalController(logger *zap.Logger, command string, arguments []string, allowedHostnames []string) (*TerminalController, error) {
	cmd := exec.Command(command, arguments...)
	tty, err := pty.Start(cmd)
	if err != nil {
		logger.Warn("failed to start command", zap.Error(err))
		return nil, err
	}
	ttyBuffer := bytes.Buffer{}
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
		historyBuffer:               ttyBuffer,
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

	controller.logger.Info("setting up new terminal connection...")

	connection, err := controller.setupConnection(ctx, ctx.Writer, ctx.Request)
	defer func(connection *websocket.Conn) {
		controller.logger.Info("closing websocket connection...")
		err := connection.Close()
		if err != nil {
			controller.logger.Warn("failed to close connection", zap.Error(err))
		}
	}(connection)
	if err != nil {
		controller.logger.Warn("failed to setup connection", zap.Error(err))
		return
	}

	// restore history from buffer
	controller.writeMutex.Lock()
	if err := connection.WriteMessage(websocket.BinaryMessage, controller.historyBuffer.Bytes()); err != nil {
		controller.logger.Info("failed to write tty buffer to xterm.js", zap.Error(err))
	}
	controller.writeMutex.Unlock()

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
	defer func() {
		waiter.Done()
		controller.logger.Info("keepAlive goroutine exiting...")
	}()
	lastPongTime := time.Now()
	keepalivePingTimeout := controller.KeepalivePingTimeout

	connection.SetPongHandler(func(msg string) error {
		lastPongTime = time.Now()
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			controller.logger.Info("done keeping alive...")
			return
		default:
			controller.logger.Info("sending keepalive ping message...")
			controller.writeMutex.Lock()
			if err := connection.WriteMessage(websocket.PingMessage, []byte("keepalive")); err != nil {
				controller.writeMutex.Unlock()
				controller.logger.Error("failed to write ping message", zap.Error(err))
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					controller.cancelFunc()
				}
				return
			}
			controller.writeMutex.Unlock()

			time.Sleep(keepalivePingTimeout / 2)

			if time.Now().Sub(lastPongTime) > keepalivePingTimeout {
				controller.logger.Warn("failed to get response from ping, triggering disconnect now...")
				return
			}
			controller.logger.Info("received response from ping successfully")
		}
	}
}

func (controller *TerminalController) readFromTTY(ctx context.Context, connection *websocket.Conn, waiter *sync.WaitGroup) {
	defer func() {
		waiter.Done()
		controller.logger.Info("readFromTTY goroutine exiting...")
	}()
	errorCounter := 0
	buffer := make([]byte, controller.MaxBufferSizeBytes)
	for {
		select {
		case <-ctx.Done():
			controller.logger.Info("done reading from tty...")
			return
		default:

			readLength, err := controller.tty.Read(buffer)
			if err != nil {
				controller.logger.Warn("failed to read from tty", zap.Error(err))
				controller.writeMutex.Lock()
				if err := connection.WriteMessage(websocket.TextMessage, []byte("bye!")); err != nil {
					controller.logger.Warn("failed to send termination message from tty to xterm.js", zap.Error(err))
				}
				controller.writeMutex.Unlock()
				return
			}

			controller.writeMutex.Lock()
			// save to history buffer
			controller.historyBuffer.Write(buffer[:readLength])
			if err := connection.WriteMessage(websocket.BinaryMessage, buffer[:readLength]); err != nil {
				controller.writeMutex.Unlock()
				controller.logger.Warn(fmt.Sprintf("failed to send %v bytes from tty to xterm.js", readLength), zap.Int("read_length", readLength), zap.Error(err))
				errorCounter++
				if errorCounter > controller.ConnectionErrorLimit {
					return
				}
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					controller.logger.Info("WebSocket closed by client")
					controller.cancelFunc()
					return
				}
				continue
			}
			controller.writeMutex.Unlock()

			controller.logger.Info(fmt.Sprintf("sent message of size %v bytes from tty to xterm.js", readLength), zap.Int("read_length", readLength))
			errorCounter = 0
		}
	}
}

func (controller *TerminalController) writeToTTY(ctx context.Context, connection *websocket.Conn, waiter *sync.WaitGroup) {
	defer func() {
		waiter.Done()
		controller.logger.Info("writeToTTY goroutine exiting...")
	}()
	for {
		select {
		case <-ctx.Done():
			controller.logger.Info("done writing from tty...")
			return
		default:

			messageType, data, err := connection.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					controller.logger.Info("WebSocket closed by client")
					controller.cancelFunc()
					return
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
				controller.logger.Error(fmt.Sprintf("failed to write %v bytes to tty: %s", len(dataBuffer), err), zap.Int("bytes_written", bytesWritten), zap.Error(err))
				continue
			}
			controller.logger.Info("bytes written to tty...", zap.Int("bytes_written", bytesWritten))
		}
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
