package discordgowrap

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"runtime"

	"github.com/gorilla/websocket"
)

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
				OS:      runtime.GOOS,
				Browser: "discordgowrap (https://github.com/skarkii/discordgowrap)",
				Device:  "discordgowrap (https://github.com/skarkii/discordgowrap)",
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
