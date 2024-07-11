package controllers

import (
	"ai-developer/app/utils"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/creack/pty"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"go.uber.org/zap"
	"os"
	"os/exec"
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
	Command                     string
	Arguments                   []string
	AllowedHostnames            []string
	logger                      *zap.Logger
}

func NewTerminalController(logger *zap.Logger, command string, arguments []string, allowedHostnames []string) *TerminalController {
	return &TerminalController{
		DefaultConnectionErrorLimit: 10,
		MaxBufferSizeBytes:          1024,
		KeepalivePingTimeout:        20 * time.Second,
		ConnectionErrorLimit:        10,
		Command:                     command,
		Arguments:                   arguments,
		AllowedHostnames:            allowedHostnames,
		logger:                      logger,
	}
}

func (controller *TerminalController) NewTerminal(ctx *gin.Context) {

	connectionErrorLimit := controller.ConnectionErrorLimit
	if connectionErrorLimit < 0 {
		connectionErrorLimit = controller.DefaultConnectionErrorLimit
	}
	maxBufferSizeBytes := controller.MaxBufferSizeBytes
	keepalivePingTimeout := controller.KeepalivePingTimeout
	if keepalivePingTimeout <= time.Second {
		keepalivePingTimeout = 20 * time.Second
	}

	allowedHostnames := controller.AllowedHostnames
	upgrader := utils.GetConnectionUpgrader(allowedHostnames, maxBufferSizeBytes)
	connection, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		controller.logger.Warn("failed to upgrade connection: %s", zap.Error(err))
		return
	}

	terminal := controller.Command
	args := controller.Arguments
	controller.logger.Debug("starting new tty", zap.String("command", terminal), zap.Strings("arguments", args))
	cmd := exec.Command(terminal, args...)
	cmd.Env = os.Environ()
	tty, err := pty.Start(cmd)
	if err != nil {
		message := fmt.Sprintf("failed to start tty: %s", err)
		controller.logger.Warn(message)
		err := connection.WriteMessage(websocket.TextMessage, []byte(message))
		if err != nil {
			controller.logger.Warn("failed to write message to connection", zap.Error(err))
			return
		}
		return
	}
	defer func() {
		controller.logger.Info("gracefully stopping spawned tty...")
		if err := cmd.Process.Kill(); err != nil {
			controller.logger.Warn("failed to kill process: %s", zap.Error(err))
		}
		if _, err := cmd.Process.Wait(); err != nil {
			controller.logger.Warn("failed to wait for process to exit: %s", zap.Error(err))
		}
		if err := tty.Close(); err != nil {
			controller.logger.Warn("failed to close spawned tty gracefully: %s", zap.Error(err))
		}
		if err := connection.Close(); err != nil {
			controller.logger.Warn("failed to close webscoket connection: %s", zap.Error(err))
		}
	}()

	var connectionClosed bool
	var waiter sync.WaitGroup
	waiter.Add(1)

	// this is a keep-alive loop that ensures connection does not hang up itself
	lastPongTime := time.Now()
	connection.SetPongHandler(func(msg string) error {
		lastPongTime = time.Now()
		return nil
	})
	go func() {
		for {
			if err := connection.WriteMessage(websocket.PingMessage, []byte("keepalive")); err != nil {
				controller.logger.Warn("failed to write ping message")
				return
			}
			time.Sleep(keepalivePingTimeout / 2)
			if time.Now().Sub(lastPongTime) > keepalivePingTimeout {
				controller.logger.Warn("failed to get response from ping, triggering disconnect now...")
				waiter.Done()
				return
			}
			controller.logger.Debug("received response from ping successfully")
		}
	}()

	// tty >> xterm.js
	go func() {
		errorCounter := 0
		for {
			// consider the connection closed/errored out so that the socket handler
			// can be terminated - this frees up memory so the service doesn't get
			// overloaded
			if errorCounter > connectionErrorLimit {
				waiter.Done()
				break
			}
			buffer := make([]byte, maxBufferSizeBytes)
			readLength, err := tty.Read(buffer)
			if err != nil {
				controller.logger.Warn("failed to read from tty", zap.Error(err))
				if err := connection.WriteMessage(websocket.TextMessage, []byte("bye!")); err != nil {
					controller.logger.Warn("failed to send termination message from tty to xterm.js", zap.Error(err))
				}
				waiter.Done()
				return
			}
			if err := connection.WriteMessage(websocket.BinaryMessage, buffer[:readLength]); err != nil {
				controller.logger.Warn("failed to send %v bytes from tty to xterm.js", zap.Int("read_length", readLength))
				errorCounter++
				continue
			}
			controller.logger.Info("sent message of size %v bytes from tty to xterm.js", zap.Int("read_length", readLength))
			errorCounter = 0
		}
	}()

	// tty << xterm.js
	go func() {
		for {
			// data processing
			messageType, data, err := connection.ReadMessage()
			if err != nil {
				if !connectionClosed {
					controller.logger.Warn("failed to get next reader", zap.Error(err))
				}
				return
			}
			dataLength := len(data)
			dataBuffer := bytes.Trim(data, "\x00")
			dataType, ok := WebsocketMessageType[messageType]
			if !ok {
				dataType = "uunknown"
			}
			controller.logger.Info(fmt.Sprintf("received %s (type: %v) message of size %v byte(s) from xterm.js with key sequence: %v", dataType, messageType, dataLength, dataBuffer))

			// process
			if dataLength == -1 { // invalid
				controller.logger.Warn("failed to get the correct number of bytes read, ignoring message")
				continue
			}

			// handle resizing
			if messageType == websocket.BinaryMessage {
				if dataBuffer[0] == 1 {
					ttySize := &TTYSize{}
					resizeMessage := bytes.Trim(dataBuffer[1:], " \n\r\t\x00\x01")
					if err := json.Unmarshal(resizeMessage, ttySize); err != nil {
						controller.logger.Warn("failed to unmarshal received resize message '%s': %s", zap.ByteString("resizeMessage", resizeMessage), zap.Error(err))
						continue
					}
					controller.logger.Info("resizing tty ", zap.Uint16("rows", ttySize.Rows), zap.Uint16("cols", ttySize.Cols))
					if err := pty.Setsize(tty, &pty.Winsize{
						Rows: ttySize.Rows,
						Cols: ttySize.Cols,
					}); err != nil {
						controller.logger.Warn("failed to resize tty", zap.Error(err))
					}
					continue
				}
			}

			// write to tty
			bytesWritten, err := tty.Write(dataBuffer)
			if err != nil {
				controller.logger.Warn(fmt.Sprintf("failed to write %v bytes to tty: %s", len(dataBuffer), err))
				continue
			}
			controller.logger.Info("bytes written to tty...", zap.Int("bytes_written", bytesWritten))
		}
	}()

	waiter.Wait()
	controller.logger.Info("closing connection...")
	connectionClosed = true

}