// Handles voice connections
package discordgowrap

import (
	"fmt"
	"log"
	"net"

	"github.com/gorilla/websocket"
)

type voiceConnection struct {
	token     string
	guildId   string
	sessionId string
	endpoint  string
	channelID string
	conn      *websocket.Conn
	intents   int
	ready     bool
	udpConn   *net.UDPConn
}

func (v *voiceConnection) SetSpeaking(speaking bool) bool {
	if v.conn == nil {
		log.Printf("[VC] No conn is available for guild %s\n", v.guildId)
		return false
	}
	fmt.Println("VC Session: ", v.sessionId, "Channel: ", v.channelID, "Endpoint: ", v.endpoint, "Speaking: ", speaking, "")
	speakingData := GatewayPayload{
		Op:   5,
		Data: SpeakingPayload{Speaking: 1, Delay: 0, Ssrc: 1},
	}
	if err := v.conn.WriteJSON(speakingData); err != nil {
		log.Printf("Error sending SPEAKING for voice channel: %v\n", err)
		return false
	}
	log.Printf("[VC] Sent SPEAKING for guild %s\n", v.guildId)
	return true
}

func (v *voiceConnection) closeVoiceSocketConnection() {
	disc := GatewayPayload{
		Op:   OpClose,
		Data: nil,
	}

	if v.conn != nil {
		log.Printf("voice conn is nil")
		return
	}

	err := v.conn.WriteJSON(disc)
	if err != nil {
		log.Printf("Error sending DISCONNECT for voice channel: %v\n", err)
	}
}

func (v *voiceConnection) establishVoiceSocketConnection() {
	if v.conn != nil {
		_ = v.conn.Close()
		v.ready = false
	}
	dialer := websocket.DefaultDialer
	v.conn, _, _ = dialer.Dial(gateway, nil)

	identify := GatewayPayload{
		Op: OpIdentify,
		Data: Identify{
			Token:   v.token,
			Intents: v.intents,
			Properties: IdentifyProperties{
				OS:      "Windows 11",
				Browser: "DiscordMusicGo",
				Device:  "DiscordMusicGo",
			},
		},
	}
	if err := v.conn.WriteJSON(identify); err != nil {
		log.Printf("Error sending IDENTIFY for voice channel: %v\n", err)
	}

	go func() {
		for {
			var payload GatewayPayload
			if err := v.conn.ReadJSON(&payload); err != nil {
				log.Printf("Failed to read voice payload: %v\n", err)
			}

			fmt.Println("[VC]", v.guildId, "Type: ", payload.Type, "Op: ", payload.Op, " Data: ", payload.Data, "")
			switch payload.Op {
			case OpHello:
				v.ready = true
				data := payload.Data.(map[string]interface{})
				heartbeatInterval := int(data["heartbeat_interval"].(float64))
				go startHeartbeat(v.conn, heartbeatInterval)

			case OpDispatch:
				continue
			}
		}
	}()
}
