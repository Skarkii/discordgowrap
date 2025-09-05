// Handles the session
package discordgowrap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

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

type SpeakingPayload struct {
	Speaking int `json:"speaking"`
	Delay    int `json:"delay"`
	Ssrc     int `json:"ssrc"`
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
	Seq  *int64      `json:"s"`
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

type VoiceServerUpdate struct {
	Endpoint string `json:"endpoint"`
	GuildId  string `json:"guild_id"`
	Token    string `json:"token"`
}

type VoiceStateUpdate struct {
	GuildId   string `json:"guild_id"`
	ChannelId string `json:"channel_id"`
	SessionId string `json:"session_id"`
	Uid       string `json:"user_id"`
}

type sendMessage struct {
	Content string `json:"content"`
}

func (s *Session) GetMessage() (string, MessageCreate, error) {
	var msg MessageCreate
	var payload GatewayPayload
	if err := s.conn.ReadJSON(&payload); err != nil {
		return "", msg, err
	}

	// Ignore heartbeat ACKs for now
	if payload.Op == OpHeartbeatACK {
		return "", msg, nil
	}
	if payload.Op == OpReconnect {
		// Implement reconnect here
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
		fmt.Printf("Voice state update: %v\n", payload.Data)
		var vsu VoiceStateUpdate
		data, _ := json.Marshal(payload.Data)
		if err := json.Unmarshal(data, &vsu); err != nil {
			return payload.Type, msg, err
		}
		if vsu.Uid != s.Bot.ID {
			fmt.Printf("Ignoring voice state update for user %s\n", vsu.Uid)
			return payload.Type, msg, nil
		}
		if vsu.ChannelId == "" {
			fmt.Printf("Ignoring leave voice state update for user\n")
			return payload.Type, msg, nil
		}
		vc := s.getVoiceConnection(vsu.GuildId)
		vc.sessionId = vsu.SessionId
		vc.channelID = vsu.ChannelId
		vc.uid = vsu.Uid
		return payload.Type, msg, nil
	case TypeVoiceServerUpdate:
		fmt.Printf("Voice server update: %v\n", payload.Data)
		var vsu VoiceServerUpdate
		data, _ := json.Marshal(payload.Data)
		if err := json.Unmarshal(data, &vsu); err != nil {
			return payload.Type, msg, err
		}
		vc := s.getVoiceConnection(vsu.GuildId)
		vc.endpoint = fmt.Sprintf("wss://%s", vsu.Endpoint)
		vc.token = vsu.Token

		vc.establishVoiceSocketConnection()

		return payload.Type, msg, nil
	}
	log.Printf("Unhandled message type: %s with op %d data: %v\n", payload.Type, payload.Op, payload.Data)
	return payload.Type, msg, nil
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
	//fmt.Println("[S] Find user response: ", respBody)

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
		log.Printf("[S] Error sending VOICE_STATE_UPDATE: %v\n", err)
	}

}
func (s *Session) Exit() error {
	// Keep this for now if more things are needed to close.
	return s.disconnect()
}

func (s *Session) SetSpeakingWrapperTest(guildId string, speaking bool) bool {
	vc := s.getVoiceConnection(guildId)
	if vc == nil {
		log.Printf("No voice connection found for guild %s", guildId)
		return false
	}
	return vc.SetSpeaking(speaking)
}

func (s *Session) PlayAudioWrapperTest(guildId string, filename string) {
	vc := s.getVoiceConnection(guildId)
	if vc == nil {
		// Here we should join the voice channel if it doesn't exist
		log.Printf("No voice connection found for guild %s", guildId)
		return
	}

	vc.playAudio(filename)
}

// Either gets the existing connection or creates a new one if it doesn't exist
func (s *Session) getVoiceConnection(guildId string) *voiceConnection {
	if voice, exists := s.voiceConnections[guildId]; exists {
		fmt.Printf("[S] Found voice connection for guild %s\n", guildId)
		//log.Println("[S] Voice channel Conn:", voice.conn)
		return voice
	}
	fmt.Printf("[S] Creating new voice connection for %s\n", guildId)
	vc := voiceConnection{
		guildId:   guildId,
		sessionId: "",
		endpoint:  "",
		channelID: "",
		uid:       "",
		conn:      nil,
		token:     s.Token,
		ssrc:      0,
	}
	s.voiceConnections[guildId] = &vc
	return &vc
}

func (s *Session) DisconnectFromVoice(guildId string) {
	fmt.Printf("All voice connections: %v\n", s.voiceConnections)
	//vc := s.getVoiceConnection(guildId)
	//vc.closeVoiceSocketConnection()

	disc := GatewayPayload{
		Op:   OpVoiceStateUpdate,
		Data: voiceChannelPost{&guildId, nil, true, false},
	}

	err := s.conn.WriteJSON(disc)
	if err != nil {
		log.Printf("[VC] Error sending DISCONNECT for voice channel: %v\n", err)
	}

	delete(s.voiceConnections, guildId)
	fmt.Printf("All voice connections: %v\n", s.voiceConnections)
}

func (s *Session) disconnect() error {
	// Closes the connection server side
	disc := GatewayPayload{
		Op:   OpClose,
		Data: nil,
	}
	return s.conn.WriteJSON(disc)
}
