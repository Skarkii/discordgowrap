// Websocket functions
package discordgowrap

import (
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

func startHeartbeat(conn *websocket.Conn, interval int) {
	ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		payload := GatewayPayload{Op: OpHeartbeat, Data: nil}
		if err := conn.WriteJSON(payload); err != nil {
			fmt.Printf("[Websocket] Error sending heartbeat: %v\n", err)
			return
		}
	}
}

func voiceStartHeartbeat(conn *websocket.Conn, interval int) {
	ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		payload := GatewayPayload{Op: OpVoiceHeartbeat, Data: nil}
		if err := conn.WriteJSON(payload); err != nil {
			fmt.Printf("[Websocket] Error sending heartbeat: %v\n", err)
			return
		}
		fmt.Println("[VC Websocket] Sent heartbeat")
	}
}
