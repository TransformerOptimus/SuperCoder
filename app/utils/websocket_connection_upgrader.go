package utils

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

func GetConnectionUpgrader(
	allowedHostnames []string,
	maxBufferSizeBytes int,
) websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			requesterHostname := r.Host
			if strings.Index(requesterHostname, ":") != -1 {
				requesterHostname = strings.Split(requesterHostname, ":")[0]
			}
			for _, allowedHostname := range allowedHostnames {
				if strings.HasSuffix(requesterHostname, allowedHostname) {
					return true
				}
			}
			fmt.Printf("failed to find '%s' in the list of allowed hostnames ('%s')\n", requesterHostname)
			return false
		},
		HandshakeTimeout: 0,
		ReadBufferSize:   maxBufferSizeBytes,
		WriteBufferSize:  maxBufferSizeBytes,
	}
}
