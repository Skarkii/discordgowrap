# discordgowrap

A minimal Go library to build Discord bots. It wraps the Discord Gateway (v10) and REST API for a straightforward, no-frills developer experience.

Status: early/experimental

- Go 1.25+
- Gateway over WebSocket with heartbeats
- Simple event reading loop
- Send channel messages (REST)
- Basic voice join scaffolding (experimental)

## Features

- Connect and identify to the Discord Gateway (v10)
- Heartbeat handling
- Event consumption via a simple polling method
- Send text messages via REST
- Experimental voice helpers:
  - Connect to a user’s current voice channel
  - Set “speaking” on the voice gateway
  - Disconnect from voice

## Installation

```shell script
go get github.com/skarkii/discordgowrap
```


## Quick start

```textmate
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/skarkii/discordgowrap"
)

func main() {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("DISCORD_TOKEN is not set")
	}

	intents := discordgowrap.IntentGuildMessages |
		discordgowrap.IntentDirectMessages |
		discordgowrap.IntentGuildVoiceStates |
		discordgowrap.IntentGuilds |
		discordgowrap.IntentMessageContent // requires privileged intent

	s, err := discordgowrap.New(token, intents)
	if err != nil {
		log.Fatalf("failed to create session: %v", err)
	}

	// Message/event loop
	go func() {
		for {
			typ, msg, err := s.GetMessage()
			if err != nil {
				log.Printf("gateway read error: %v", err)
				continue
			}

			// Handle just a couple of cases
			switch typ {
			case discordgowrap.TypeMessageCreate:
				// Ignore self
				if msg.Author.ID == s.Bot.ID {
					continue
				}

				fmt.Printf("[%s] %s\n", msg.Author.Name, msg.Content)

				if msg.Content == "-ping" {
					if err := s.SendMessage(msg.ChannelID, "Pong!"); err != nil {
						log.Printf("send error: %v", err)
					}
				}
				if msg.Content == "-play" {
					// Joins the author's current voice channel (experimental)
					s.ConnectToVoice(msg.GuildID, msg.Author.ID)
				}
				if msg.Content == "-stop" {
					s.DisconnectFromVoice(msg.GuildID)
				}
				if msg.Content == "-speak" {
					// Toggle speaking on the voice gateway (experimental)
					s.SetSpeakingWrapperTest(msg.GuildID, true)
				}
			default:
				// Unhandled events are logged inside the library
			}
		}
	}()

	fmt.Printf("Bot %q is now running! Press Ctrl+C to exit.\n", s.Bot.Name)

	// Graceful shutdown
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	if err := s.Exit(); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	fmt.Println("Bot shutdown gracefully")
}
```


## Notes on voice (experimental)

- Voice support is in-progress and subject to breaking changes.
- It currently:
  - Triggers a voice state update to join the author’s current channel.
  - Opens a voice WebSocket and can send “speaking” frames.
  - Contains initial UDP connection scaffolding.
- It does not yet implement full audio send/receive, encryption, or robust reconnection.

## Caveats and roadmap

- Minimal error handling and no rate-limit backoff for REST.
- No auto-reconnect/resume, sharding, or command framework.
- Event model is a simple polling loop via GetMessage.
- Voice support is experimental and incomplete.

Planned improvements:
- Robust reconnect/resume and heartbeat ACK handling
- Proper rate limit handling
- Event dispatcher with handlers
- Full voice support (encryption, audio send/receive)

## Contributing

Issues and PRs are welcome. Please include:
- A clear description of the problem/feature
- Minimal reproduction (if applicable)

If you have any questions or need guidance, open an issue.
