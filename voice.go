// Handles voice connections
package discordgowrap

import (
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/gorilla/websocket"
)

const (
	// Code	Name	Sent By	Description	Binary
	OpVoiceIdentify                        = 0  // client	Begin a voice websocket connection.
	OpVoiceSelectProtocol                  = 1  // client	Select the voice protocol.
	OpVoiceReady                           = 2  // server	Complete the websocket handshake.
	OpVoiceHeartbeat                       = 3  // client	Keep the websocket connection alive.
	OpVoiceSessionDescription              = 4  // server	Describe the session.
	OpVoiceSpeaking                        = 5  // client/server	Indicate which users are speaking.
	OpVoiceHeartbeatAck                    = 6  // server	Sent to acknowledge a received client heartbeat.
	OpVoiceResume                          = 7  // client	Resume a connection.
	OpVoiceHello                           = 8  // server	Time to wait between sending heartbeats in milliseconds.
	OpVoiceResumed                         = 9  // server	Acknowledge a successful session resume.
	OpVoiceClientsConnect                  = 11 // server	One or more clients have connected to the voice channel
	OpVoiceClientDisconnect                = 13 // server	A client has disconnected from the voice channel
	OpVoiceDAVEPrepareTransition           = 21 // server	A downgrade from the DAVE protocol is upcoming
	OpVoiceDAVEExecuteTransition           = 22 // server	Execute a previously announced protocol transition
	OpVoiceDAVETransitionReady             = 23 // client	Acknowledge readiness previously announced transition
	OpVoiceDAVEPrepareEpoch                = 24 // server	A DAVE protocol version or group change is upcoming
	OpVoiceDAVEMLSExternalSender           = 25 // server	Credential and public key for MLS external sender	X
	OpVoiceDAVEMLSKeyPackage               = 26 // client	MLS Key Package for pending group member	X
	OpVoiceDAVEMLSProposals                = 27 // server	MLS Proposals to be appended or revoked	X
	OpVoiceDAVEMLSCommitWelcome            = 28 // client	MLS Commit with optional MLS Welcome messages	X
	OpVoiceDAVEMLSAnnounceCommitTransition = 29 // server	MLS Commit to be processed for upcoming transition	X
	OpVoiceDAVEMLSWelcome                  = 30 // server	MLS Welcome to group for upcoming transition	X
	OpVoiceDAVEMLSInvalidCommitWelcome     = 31 // client	Flag invalid commit or welcome, request re-add
)

const (
	//	Code	Description	Explanation
	//
	// 4001	Unknown opcode	You sent an invalid opcode.
	// 4002	Failed to decode payload	You sent an invalid payload in your identifying to the Gateway.
	// 4003	Not authenticated	You sent a payload before identifying with the Gateway.
	// 4004	Authentication failed	The token you sent in your identify payload is incorrect.
	// 4005	Already authenticated	You sent more than one identify payload. Stahp.
	// 4006	Session no longer valid	Your session is no longer valid.
	// 4009	Session timeout	Your session has timed out.
	// 4011	Server not found	We can't find the server you're trying to connect to.
	// 4012	Unknown protocol	We didn't recognize the protocol you sent.
	// 4014	Disconnected	Disconnect individual client (you were kicked, the main gateway session was dropped, etc.). Should not reconnect.
	// 4015	Voice server crashed	The server crashed. Our bad! Try resuming.
	// 4016	Unknown encryption mode	We didn't recognize your encryption.
	// 4020	Bad request	You sent a malformed request
	// 4021	Disconnected: Rate Limited	Disconnect due to rate limit exceeded. Should not reconnect.
	// 4022	Disconnected: Call Terminated	Disconnect all clients due to call terminated (channel deleted, voice server changed, etc.). Should not reconnect.
	VoiceDisconnected = 4022
)

type voiceConnection struct {
	token      string
	guildId    string
	sessionId  string
	endpoint   string
	uid        string
	channelID  string
	conn       *websocket.Conn
	connWmutex sync.Mutex
	intents    int
	ready      bool
	udpConn    *net.UDPConn
}

type voiceChannelPost struct {
	GuildID   *string `json:"guild_id"`
	ChannelID *string `json:"channel_id"`
	SelfMute  bool    `json:"self_mute"`
	SelfDeaf  bool    `json:"self_deaf"`
}

type voiceChannelSpeaking struct {
	Speaking bool `json:"speaking"`
	Delay    int  `json:"delay"`
}

func (v *voiceConnection) SetSpeaking(speaking bool) bool {
	if v.conn == nil {
		log.Printf("[VC] No conn is available for guild %s\n", v.guildId)
		return false
	}
	//fmt.Println("VC Session: ", v.sessionId, "Channel: ", v.channelID, "Endpoint: ", v.endpoint, "Speaking: ", speaking, "")
	speakingData := GatewayPayload{
		Op:   5,
		Data: voiceChannelSpeaking{speaking, 0},
	}
	v.connWmutex.Lock()
	defer v.connWmutex.Unlock()
	if err := v.conn.WriteJSON(speakingData); err != nil {
		log.Printf("[VC] Error sending SPEAKING for voice channel: %v\n", err)
		return false
	}
	fmt.Printf("[VC] Sent SPEAKING %d for guild %s\n", speaking, v.guildId)
	return true
}

func (v *voiceConnection) closeVoiceSocketConnection() {
	fmt.Println("[VC] Closing voice connection for guild:", v.guildId)
	disc := GatewayPayload{
		Op:   OpVoiceStateUpdate,
		Data: voiceChannelPost{&v.guildId, nil, true, false},
	}
	fmt.Printf("disc: %v\n", disc)

	if v.conn == nil {
		log.Println("[VC] Conn is nil")
		return
	}

	v.connWmutex.Lock()
	defer v.connWmutex.Unlock()
	err := v.conn.WriteJSON(disc)
	if err != nil {
		log.Printf("[VC] Error sending DISCONNECT for voice channel: %v\n", err)
	}
}

type VoiceIdentify struct {
	ServerID string `json:"server_id"`
	UserID   string `json:"user_id"`
	Session  string `json:"session_id"`
	Token    string `json:"token"`
}

func (v *voiceConnection) establishVoiceSocketConnection() {
	if v.conn != nil {
		_ = v.conn.Close()
		v.ready = false
	}
	dialer := websocket.DefaultDialer
	var err error
	v.conn, _, err = dialer.Dial(v.endpoint, nil)

	if err != nil {
		log.Printf("[VC] Error establishing voice connection: %v\n", err)
		return
	}

	//fmt.Printf("[VC] Establishing voice connection for guild %s, session %s, userid %s, token %s\n", v.guildId, v.sessionId, v.token, v.token)
	identify := GatewayPayload{
		Op: OpVoiceIdentify,
		Data: VoiceIdentify{
			ServerID: v.guildId,
			UserID:   v.uid,
			Session:  v.sessionId,
			Token:    v.token,
		},
	}
	fmt.Printf("[VC] Identify: %v\n", identify)

	v.connWmutex.Lock()
	if err := v.conn.WriteJSON(identify); err != nil {
		log.Printf("Error sending IDENTIFY for voice channel: %v\n", err)
	}
	v.connWmutex.Unlock()

	go func() {
		for {
			var payload GatewayPayload
			if err := v.conn.ReadJSON(&payload); err != nil {
				if websocket.IsCloseError(err, 4014) {
					break
				}
				log.Printf("[VC]Failed to read voice payload: %v\n", err)
				break
			}

			//fmt.Println("[VC]", v.guildId, "Type: ", payload.Type, "Op: ", payload.Op, " Data: ", payload.Data, "")
			//fmt.Println("[VC] Received payload: ", payload)
			fmt.Println("[VC] Received payload: OP", payload.Op, "Seq: ", payload.Seq, "Data:", payload.Data, "")
			switch payload.Op {
			case OpVoiceHello:
				data := payload.Data.(map[string]interface{})
				heartbeatInterval := int(data["heartbeat_interval"].(float64))
				go v.voiceStartHeartbeat(heartbeatInterval)
			case OpVoiceHeartbeatAck:
				fmt.Println("[VC] Received heartbeat ack")
			case OpVoiceReady:
				fmt.Println("[VC] Received READY")
				v.ready = true
			case OpVoiceClientDisconnect:
				fmt.Println("[VC] Received CLIENT DISCONNECT")
			}
		}
		fmt.Println("[VC] Voice connection closed")
	}()
}
