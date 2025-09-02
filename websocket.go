// Websocket functions
package discordgowrap

import (
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

func startHeartbeat(conn *websocket.Conn, interval int) {
	interval = interval - (interval / 20)
	fmt.Println("[Websocket] starting heartbeat with interval:", interval, "ms for Session")
	ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		payload := GatewayPayload{Op: OpHeartbeat, Data: nil}
		if err := conn.WriteJSON(payload); err != nil {
			fmt.Printf("[Websocket] Error sending heartbeat: %v\n", err)
			return
		}
		//fmt.Println("[Websocket] Sent heartbeat to:", conn.RemoteAddr())
	}
}

func (v *voiceConnection) voiceStartHeartbeat(interval int) {
	interval = interval - (interval / 10)
	fmt.Println("[VC Websocket] starting heartbeat with interval:", interval, "ms", "for guild:", v.guildId)
	ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
	var seq int64 = 0
	defer ticker.Stop()
	for range ticker.C {
		payload := GatewayPayload{Op: OpVoiceHeartbeat, Data: seq}
		v.connWmutex.Lock()
		if err := v.conn.WriteJSON(payload); err != nil {
			fmt.Printf("[VC Websocket] Error sending heartbeat: %v\n", err)
			return
		}
		v.connWmutex.Unlock()
		//fmt.Println("[VC Websocket] Sent heartbeat with seq:", seq, "to:", conn.RemoteAddr())
		seq += 1
	}
}
