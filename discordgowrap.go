/*
Custom Discord API using websockets. Main focus here is parallel voice channel connections
*/
package discordgowrap

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type Session struct {
	Token      string
	conn       *websocket.Conn
	intents    int
	Bot        bot
	httpClient *http.Client
}

const (
	// https://discord-intents-calculator.vercel.app/
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

const (
	// https://discord.com/developers/docs/topics/opcodes-and-status-codes#gateway-gateway-opcodes
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

func (s Session) GetMessage() (string, MessageCreate, error) {
	var msg MessageCreate
	var payload GatewayPayload
	if err := s.conn.ReadJSON(&payload); err != nil {
		return "", msg, err
	}
	if payload.Type == "MESSAGE_CREATE" {
		data, _ := json.Marshal(payload.Data)
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", msg, err
		}
	}
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
		log.Printf("Error sending IDENTIFY: %v\n", err)
	}

	var payload GatewayPayload
	if err := conn.ReadJSON(&payload); err != nil {
		return nil, err
	}

	if payload.Op != OpHello {
		return nil, errors.New("Invalid starting operator retrieved!")
	}
	data := payload.Data.(map[string]interface{})
	heartbeatInterval := int(data["heartbeat_interval"].(float64))

	for {
		if err := conn.ReadJSON(&payload); err != nil {
			return nil, err
		}
		if payload.Type == "READY" {
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

type SendMessage struct {
	Content string `json:"content"`
}

func (s Session) SendMessage(channelID string, content string) error {
	fmt.Printf("Sending \"%s\"to channel %s\n", content, channelID)
	url := fmt.Sprintf("%s/channels/%s/messages", apiBase, channelID)
	msg := SendMessage{Content: content}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("error marshaling message: %v", err)
	}
	return s.httpRequestNoResponse("POST", url, body)
}

func (s Session) httpRequestAndResponse(method string, url string, body []byte) (string, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bot "+s.Token)
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		return "", err
	}

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

func (s Session) httpRequestNoResponse(method string, url string, body []byte) error {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bot "+s.Token)
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (s Session) findUserChannelIdInGuild(guildId string, userId string) string {
	type voiceState struct {
		ChannelID string `json:"channel_id"`
	}

	url := fmt.Sprintf("%s/guilds/%s/voice-states/%s", apiBase, guildId, userId)

	respBody, err := s.httpRequestAndResponse("GET", url, nil)
	if err != nil {
		log.Printf("findUserChannelIdInGuild: request error: %v\n", err)
		return ""
	}
	fmt.Println(respBody)

	var vs voiceState
	if err := json.Unmarshal([]byte(respBody), &vs); err != nil {
		log.Printf("findUserChannelIdInGuild: unmarshal error: %v\nBody: %s\n", err, respBody)
		return ""
	}

	return vs.ChannelID
}

// https://discord.com/developers/docs/topics/voice-connections#retrieving-voice-server-information
func (s Session) ConnectToVoice(guildId string, userId string) {
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
