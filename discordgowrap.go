// Package discordgowrap
// Discord API wrapper for Go
// Github: https://www.github.com/skarkii/discordgowrap
package discordgowrap

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type Session struct {
	Token            string
	conn             *websocket.Conn
	intents          int
	Bot              bot
	httpClient       *http.Client
	voiceConnections map[string]*voiceConnection
}

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

func (s *Session) DisconnectFromVoice(guildId string) {
	fmt.Printf("All voice connections: %v\n", s.voiceConnections)
	vc := s.getVoiceConnection(guildId)
	vc.closeVoiceSocketConnection()
	//delete(s.voiceConnections, guildId)
	fmt.Printf("All voice connections: %v\n", s.voiceConnections)
}

type SpeakingPayload struct {
	Speaking int `json:"speaking"`
	Delay    int `json:"delay"`
	Ssrc     int `json:"ssrc"`
}

func (s *Session) SetSpeakingWrapperTest(guildId string, speaking bool) bool {
	vc := s.getVoiceConnection(guildId)
	if vc == nil {
		log.Printf("No voice connection found for guild %s", guildId)
		return false
	}
	return vc.SetSpeaking(speaking)
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

func (v voiceConnection) closeVoiceSocketConnection() {
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
				if payload.Type != TypeReady {
					continue
				}
				fmt.Println("[VC] Connecting with UDP for guild ", v.guildId, "with endpint", v.endpoint)
				//if err := v.establishUDPConnection(); err != nil {
				//	fmt.Println("[VCUDP]", v.guildId, "Failed to establish UDP connection: ", err)
				//}
			}
		}
	}()
}

func (v *voiceConnection) establishUDPConnection() error {
	if v.endpoint == "" {
		return errors.New("no endpoint available for UDP connection")
	}

	// Remove the port from endpoint if present and add the voice port
	host := v.endpoint
	if host[len(host)-1] == ':' {
		host = host[:len(host)-1]
	}

	// Discord voice uses port 80 for UDP
	udpAddr, err := net.ResolveUDPAddr("udp", host+":80")
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %v", err)
	}

	// Establish UDP connection
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return fmt.Errorf("failed to establish UDP connection: %v", err)
	}

	v.udpConn = conn
	log.Printf("UDP connection established to %s\n", udpAddr.String())

	// Perform IP discovery handshake
	if err := v.performIPDiscovery(); err != nil {
		return fmt.Errorf("IP discovery failed: %v", err)
	}

	// Start UDP data reading loop
	go v.readUDPData()

	return nil
}

// Add this new method to read and print UDP data
func (v *voiceConnection) readUDPData() {
	if v.udpConn == nil {
		log.Printf("UDP connection is nil, cannot start reading\n")
		return
	}

	buffer := make([]byte, 1024)

	for {
		n, err := v.udpConn.Read(buffer)
		if err != nil {
			log.Printf("Error reading UDP data for guild %s: %v\n", v.guildId, err)
			break
		}

		if n > 0 {
			fmt.Printf("UDP Data received for guild %s (%d bytes): %x\n", v.guildId, n, buffer[:n])

			// If you want to see the raw bytes as well
			fmt.Printf("UDP Data as bytes: %v\n", buffer[:n])
		}
	}

	log.Printf("UDP reading loop ended for guild %s\n", v.guildId)
}

func (v *voiceConnection) performIPDiscovery() error {
	if v.udpConn == nil {
		return errors.New("UDP connection not established")
	}

	// Create IP discovery packet
	packet := make([]byte, 70)
	// Packet type (IP Discovery)
	packet[0] = 0x00
	packet[1] = 0x01
	// Length (70 bytes)
	packet[2] = 0x00
	packet[3] = 0x46
	// SSRC (we'll use 1 for now)
	packet[4] = 0x00
	packet[5] = 0x00
	packet[6] = 0x00
	packet[7] = 0x01

	// Send IP discovery packet
	_, err := v.udpConn.Write(packet)
	if err != nil {
		return fmt.Errorf("failed to send IP discovery packet: %v", err)
	}

	// Read response
	response := make([]byte, 70)
	_, err = v.udpConn.Read(response)
	if err != nil {
		return fmt.Errorf("failed to read IP discovery response: %v", err)
	}

	// Extract IP and port from response
	ip := string(response[4:68])
	ip = ip[:len(ip)-1] // Remove null terminator
	port := int(response[68])<<8 | int(response[69])

	log.Printf("IP Discovery complete - IP: %s, Port: %d\n", ip, port)

	return nil
}

// Either gets the existing connection or creates a new one if it doesn't exist
func (s *Session) getVoiceConnection(guildId string) *voiceConnection {
	if voice, exists := s.voiceConnections[guildId]; exists {
		log.Printf("Found voice connection for guild %s\n", guildId)
		log.Println("[Voice channel] Conn exists:", voice.conn)
		return voice
	}
	log.Printf("Creating new voice connection for %s\n", guildId)
	vc := voiceConnection{
		guildId:   guildId,
		sessionId: "",
		endpoint:  "",
		channelID: "",
		conn:      nil,
		token:     s.Token,
	}
	s.voiceConnections[guildId] = &vc
	return &vc
}

// https://discord.com/developers/docs/events/gateway-events#receive-events
const (
	TypeChannelCreate                 = "CHANNEL_CREATE"
	TypeChannelUpdate                 = "CHANNEL_UPDATE"
	TypeChannelDelete                 = "CHANNEL_DELETE"
	TypeChannelPinsUpdate             = "CHANNEL_PINS_UPDATE"
	TypeThreadCreate                  = "THREAD_CREATE"
	TypeThreadUpdate                  = "THREAD_UPDATE"
	TypeThreadDelete                  = "THREAD_DELETE"
	TypeThreadListSync                = "THREAD_LIST_SYNC"
	TypeThreadMemberUpdate            = "THREAD_MEMBER_UPDATE"
	TypeThreadMembersUpdate           = "THREAD_MEMBERS_UPDATE"
	TypeGuildCreate                   = "GUILD_CREATE"
	TypeGuildUpdate                   = "GUILD_UPDATE"
	TypeGuildDelete                   = "GUILD_DELETE"
	TypeGuildBanAdd                   = "GUILD_BAN_ADD"
	TypeGuildBanRemove                = "GUILD_BAN_REMOVE"
	TypeGuildEmojisUpdate             = "GUILD_EMOJIS_UPDATE"
	TypeGuildStickersUpdate           = "GUILD_STICKERS_UPDATE"
	TypeGuildIntegrationsUpdate       = "GUILD_INTEGRATIONS_UPDATE"
	TypeGuildMemberAdd                = "GUILD_MEMBER_ADD"
	TypeGuildMemberRemove             = "GUILD_MEMBER_REMOVE"
	TypeGuildMemberUpdate             = "GUILD_MEMBER_UPDATE"
	TypeGuildMembersChunk             = "GUILD_MEMBERS_CHUNK"
	TypeGuildRoleCreate               = "GUILD_ROLE_CREATE"
	TypeGuildRoleUpdate               = "GUILD_ROLE_UPDATE"
	TypeGuildRoleDelete               = "GUILD_ROLE_DELETE"
	TypeGuildScheduledEventCreate     = "GUILD_SCHEDULED_EVENT_CREATE"
	TypeGuildScheduledEventUpdate     = "GUILD_SCHEDULED_EVENT_UPDATE"
	TypeGuildScheduledEventDelete     = "GUILD_SCHEDULED_EVENT_DELETE"
	TypeGuildScheduledEventUserAdd    = "GUILD_SCHEDULED_EVENT_USER_ADD"
	TypeGuildScheduledEventUserRemove = "GUILD_SCHEDULED_EVENT_USER_REMOVE"
	TypeIntegrationCreate             = "INTEGRATION_CREATE"
	TypeIntegrationUpdate             = "INTEGRATION_UPDATE"
	TypeIntegrationDelete             = "INTEGRATION_DELETE"
	TypeInteractionCreate             = "INTERACTION_CREATE"
	TypeInviteCreate                  = "INVITE_CREATE"
	TypeInviteDelete                  = "INVITE_DELETE"
	TypeMessageCreate                 = "MESSAGE_CREATE"
	TypeMessageUpdate                 = "MESSAGE_UPDATE"
	TypeMessageDelete                 = "MESSAGE_DELETE"
	TypeMessageDeleteBulk             = "MESSAGE_DELETE_BULK"
	TypeMessageReactionAdd            = "MESSAGE_REACTION_ADD"
	TypeMessageReactionRemove         = "MESSAGE_REACTION_REMOVE"
	TypeMessageReactionRemoveAll      = "MESSAGE_REACTION_REMOVE_ALL"
	TypeMessageReactionRemoveEmoji    = "MESSAGE_REACTION_REMOVE_EMOJI"
	TypePresenceUpdate                = "PRESENCE_UPDATE"
	TypeReady                         = "READY"
	TypeStageInstanceCreate           = "STAGE_INSTANCE_CREATE"
	TypeStageInstanceUpdate           = "STAGE_INSTANCE_UPDATE"
	TypeStageInstanceDelete           = "STAGE_INSTANCE_DELETE"
	TypeTypingStart                   = "TYPING_START"
	TypeUserUpdate                    = "USER_UPDATE"
	TypeVoiceStateUpdate              = "VOICE_STATE_UPDATE"
	TypeVoiceServerUpdate             = "VOICE_SERVER_UPDATE"
	TypeWebhooksUpdate                = "WEBHOOKS_UPDATE"
)

// https://discord-intents-calculator.vercel.app/
const (
	IntentGuilds                      = 1 << 0
	IntentGuildMembers                = 1 << 1
	IntentGuildModeration             = 1 << 2
	IntentGuildExpressions            = 1 << 3
	IntentGuildIntegrations           = 1 << 4
	IntentGuildWebhooks               = 1 << 5
	IntentGuildInvites                = 1 << 6
	IntentGuildVoiceStates            = 1 << 7
	IntentGuildPresences              = 1 << 8
	IntentGuildMessages               = 1 << 9
	IntentGuildMessageReactions       = 1 << 10
	IntentGuildMessageTyping          = 1 << 11
	IntentDirectMessages              = 1 << 12
	IntentDirectMessageReactions      = 1 << 13
	IntentDirectMessageTyping         = 1 << 14
	IntentMessageContent              = 1 << 15
	IntentGuildScheduledEvents        = 1 << 16
	IntentAutoModerationConfiguration = 1 << 20
	IntentAutoModerationExecution     = 1 << 21
	IntentGuildMessagePolls           = 1 << 24
	IntentDirectMessagePolls          = 1 << 25
)

// https://discord.com/developers/docs/topics/opcodes-and-status-codes#gateway-gateway-opcodes
const (
	OpDispatch                = 0    // receive - An event was dispatched.
	OpHeartbeat               = 1    // send/receive - Fired periodically by the client to keep the connection alive.
	OpIdentify                = 2    // send - Starts a new session during the initial handshake.
	OpPresenceUpdate          = 3    // send - Update the client's presence.
	OpVoiceStateUpdate        = 4    // send - Used to join/leave or move between voice channels.
	OpResume                  = 6    // send - Resume a previous session that was disconnected.
	OpReconnect               = 7    // receive - You should attempt to reconnect and resume immediately.
	OpRequestGuildMembers     = 8    // send - Request information about offline guild members in a large guild.
	OpInvalidSession          = 9    // receive - The session has been invalidated. You should reconnect and identify/resume accordingly.
	OpHello                   = 10   // receive - Sent immediately after connecting, contains the heartbeat_interval to use.
	OpHeartbeatACK            = 11   // receive - Sent in response to receiving a heartbeat to acknowledge that it has been received.
	OpRequestSoundboardSounds = 31   // send - Request information about soundboard sounds in a set of guilds.
	OpClose                   = 1000 // send - Send the connection
)

const (
	OpVoiceSpeaking = 5
)

const (
	gateway = "wss://gateway.discord.gg/?v=10&encoding=json"
	apiBase = "https://discord.com/api/v10"
)

type MessageCreate struct {
	Content string `json:"content"`
	Author  struct {
		ID   string `json:"id"`
		Name string `json:"username"`
	} `json:"author"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
}

type bot struct {
	ID   string `json:"id"`
	Name string `json:"username"`
}

type ReadyCreate struct {
	User struct {
		ID   string `json:"id"`
		Name string `json:"username"`
	} `json:"user"`
}

type GatewayPayload struct {
	Op   int         `json:"op"`
	Data interface{} `json:"d"`
	Seq  int         `json:"s,omitempty"`
	Type string      `json:"t,omitempty"`
}

type Identify struct {
	Token      string             `json:"token"`
	Intents    int                `json:"intents"`
	Properties IdentifyProperties `json:"properties"`
}

type IdentifyProperties struct {
	OS      string `json:"os"`
	Browser string `json:"browser"`
	Device  string `json:"device"`
}

func (s Session) Exit() error {
	// Keep this for now if more things are needed to close.
	return s.disconnect()
}

func (s Session) disconnect() error {
	// Closes the connection server side
	disc := GatewayPayload{
		Op:   OpClose,
		Data: nil,
	}
	return s.conn.WriteJSON(disc)
}

type VoiceServerUpdate struct {
	Endpoint string `json:"endpoint"`
	GuildId  string `json:"guild_id"`
	Token    string `json:"token"`
}

type VoiceStateUpdate struct {
	GuildId   string `json:"guild_id"`
	ChannelId string `json:"channel_id"`
	SessionId string `json:"session_id"`
}

func (s Session) GetMessage() (string, MessageCreate, error) {
	var msg MessageCreate
	var payload GatewayPayload
	if err := s.conn.ReadJSON(&payload); err != nil {
		return "", msg, err
	}

	// Ignore heartbeat ACKs
	if payload.Op == OpHeartbeatACK {
		return "", msg, nil
	}

	switch payload.Type {
	case TypeMessageCreate:
		data, _ := json.Marshal(payload.Data)
		if err := json.Unmarshal(data, &msg); err != nil {
			return payload.Type, msg, err
		}
		return payload.Type, msg, nil
	case TypeMessageUpdate:
		return payload.Type, msg, nil
	case TypeGuildCreate:
		return payload.Type, msg, nil
	case TypeVoiceStateUpdate:
		var vsu VoiceStateUpdate
		data, _ := json.Marshal(payload.Data)
		if err := json.Unmarshal(data, &vsu); err != nil {
			return payload.Type, msg, err
		}
		vc := s.getVoiceConnection(vsu.GuildId)
		vc.sessionId = vsu.SessionId
		vc.channelID = vsu.ChannelId
		vc.establishVoiceSocketConnection()
		return payload.Type, msg, nil
	case TypeVoiceServerUpdate:
		var vsu VoiceServerUpdate
		data, _ := json.Marshal(payload.Data)
		if err := json.Unmarshal(data, &vsu); err != nil {
			return payload.Type, msg, err
		}
		vc := s.getVoiceConnection(vsu.GuildId)
		vc.endpoint = vsu.Endpoint
		vc.token = vsu.Token

		return payload.Type, msg, nil
	}
	log.Printf("Unhandled message type: %s with data: %v\n", payload.Type, payload.Data)
	return payload.Type, msg, nil
}

func New(token string, intents int) (*Session, error) {
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.Dial(gateway, nil)

	if err != nil {
		log.Fatalf("Failed to establish connection to Discord: %v", err)
	}

	// Identifies and connects the bot
	identify := GatewayPayload{
		Op: OpIdentify,
		Data: Identify{
			Token:   token,
			Intents: intents,
			Properties: IdentifyProperties{
				OS:      "Windows 11",
				Browser: "DiscordMusicGo",
				Device:  "DiscordMusicGo",
			},
		},
	}
	if err := conn.WriteJSON(identify); err != nil {
		log.Printf("error sending IDENTIFY: %v\n", err)
	}

	var payload GatewayPayload
	if err := conn.ReadJSON(&payload); err != nil {
		return nil, err
	}

	if payload.Op != OpHello {
		return nil, errors.New("invalid starting operator retrieved")
	}
	data := payload.Data.(map[string]interface{})
	heartbeatInterval := int(data["heartbeat_interval"].(float64))

	for {
		if err := conn.ReadJSON(&payload); err != nil {
			return nil, err
		}
		if payload.Type == TypeReady {
			break
		}
	}
	// This section needs reformatting to prevent duplicate usage of "data"
	var msg ReadyCreate
	adata, _ := json.Marshal(payload.Data)
	if err := json.Unmarshal(adata, &msg); err != nil {
		log.Printf("Error unmarshaling message: %v\n", err)
	}
	s := Session{
		token,
		conn,
		intents,
		bot{ID: msg.User.ID, Name: msg.User.Name},
		&http.Client{},
		make(map[string]*voiceConnection),
	}
	//fmt.Printf("Retrieved Ack from Identify, starting heartbeat\n")
	go startHeartbeat(s.conn, heartbeatInterval)

	return &s, nil
}

func startHeartbeat(conn *websocket.Conn, interval int) {
	ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		payload := GatewayPayload{Op: OpHeartbeat, Data: nil}
		if err := conn.WriteJSON(payload); err != nil {
			fmt.Printf("Error sending heartbeat: %v\n", err)
			return
		}
	}
}

type sendMessage struct {
	Content string `json:"content"`
}

func (s *Session) SendMessage(channelID string, content string) error {
	fmt.Printf("Sending \"%s\" to channel %s\n", content, channelID)
	url := fmt.Sprintf("%s/channels/%s/messages", apiBase, channelID)
	msg := sendMessage{Content: content}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("error marshaling message: %v", err)
	}
	return s.httpRequestNoResponse("POST", url, body)
}

func (s *Session) httpRequestAndResponse(method string, url string, body []byte) (string, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bot "+s.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	recvBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(recvBody), nil
}

func (s *Session) httpRequestNoResponse(method string, url string, body []byte) error {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+s.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (s *Session) findUserChannelIdInGuild(guildId string, userId string) string {
	type voiceState struct {
		ChannelID string `json:"channel_id"`
	}

	url := fmt.Sprintf("%s/guilds/%s/voice-states/%s", apiBase, guildId, userId)

	respBody, err := s.httpRequestAndResponse("GET", url, nil)
	if err != nil {
		log.Printf("findUserChannelIdInGuild: request error: %v\n", err)
		return ""
	}
	fmt.Println("Find user response: ", respBody)

	var vs voiceState
	if err := json.Unmarshal([]byte(respBody), &vs); err != nil {
		log.Printf("findUserChannelIdInGuild: unmarshal error: %v\nBody: %s\n", err, respBody)
		return ""
	}

	return vs.ChannelID
}

func (s *Session) ConnectToVoice(guildId string, userId string) {
	// https://discord.com/developers/docs/topics/voice-connections#retrieving-voice-server-information
	channelId := s.findUserChannelIdInGuild(guildId, userId)

	if channelId == "" {
		log.Printf("Failed to find channelID\n")
		return
	}

	payload := GatewayPayload{
		Op: OpVoiceStateUpdate,
		Data: map[string]interface{}{
			"guild_id":   guildId,
			"channel_id": channelId,
			"self_mute":  false,
			"self_deaf":  true,
		},
	}

	if err := s.conn.WriteJSON(payload); err != nil {
		log.Printf("Error sending VOICE_STATE_UPDATE: %v\n", err)
	}

}
