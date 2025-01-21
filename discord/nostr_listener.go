package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

type DiscordWebhookMessage struct {
	Content string `json:"content"`
}

func listenToNostrEvents() {
	relayURL := os.Getenv("RELAY_URL")
	if relayURL == "" {
		log.Fatal("RELAY_URL environment variable is required")
	}

	discordWebhook := os.Getenv("DISCORD_WEBHOOK")
	if discordWebhook == "" {
		log.Fatal("DISCORD_WEBHOOK environment variable is required")
	}

	for {
		log.Printf("Connecting to relay %s...", relayURL)

		ctx, cancel := context.WithCancel(context.Background())
		relay, err := nostr.RelayConnect(ctx, relayURL)
		if err != nil {
			log.Printf("Failed to connect to relay: %v. Retrying in 5 seconds...", err)
			cancel()
			time.Sleep(5 * time.Second)
			continue
		}

		// Subscribe to kind 30311 events (NIP-53 Live Activities)
		timestamp := nostr.Timestamp(time.Now().Unix())
		sub, err := relay.Subscribe(ctx, []nostr.Filter{{
			Kinds: []int{30311},
			Since: &timestamp, // Pass the address of the timestamp
		}})
		if err != nil {
			log.Printf("Failed to subscribe: %v. Retrying in 5 seconds...", err)
			cancel()
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("Connected to relay %s and subscribed to NIP-53 Live Activity events (kind 30311)", relayURL)

		// Listen for events
		for event := range sub.Events {
			log.Printf("Received NIP-53 event with ID: %s", event.ID)

			// Create Discord message
			message := DiscordWebhookMessage{
				Content: formatNostrMessage(event, nil),
			}

			// Send to Discord with retries
			for retries := 0; retries < 3; retries++ {
				if err := sendToDiscord(discordWebhook, message); err != nil {
					if retries < 2 {
						log.Printf("Failed to send to Discord: %v. Retrying in 2 seconds...", err)
						time.Sleep(2 * time.Second)
						continue
					}
					log.Printf("Failed to send to Discord after 3 attempts: %v", err)
				} else {
					log.Printf("Successfully sent event %s to Discord", event.ID)
					break
				}
			}
		}

		// If we get here, the subscription was closed
		log.Printf("Subscription closed. Reconnecting in 5 seconds...")
		cancel()
		time.Sleep(5 * time.Second)
	}
}

func formatNostrMessage(event *nostr.Event, content map[string]interface{}) string {
	// Get important tags
	var title, summary, image, status, starts, ends, streaming string
	var participants []string

	// Convert pubkey to npub
	npub, _ := nip19.EncodePublicKey(event.PubKey)
	authorNpub := npub[:8] + "..." // Take first 8 chars

	for _, tag := range event.Tags {
		switch tag[0] {
		case "title":
			title = tag[1]
		case "summary":
			summary = tag[1]
		case "image":
			image = tag[1]
		case "status":
			status = tag[1]
		case "streaming":
			streaming = tag[1]
		case "starts":
			if t, err := strconv.ParseInt(tag[1], 10, 64); err == nil {
				starts = time.Unix(t, 0).Format(time.RFC1123)
			}
		case "ends":
			if t, err := strconv.ParseInt(tag[1], 10, 64); err == nil {
				ends = time.Unix(t, 0).Format(time.RFC1123)
			}
		case "p":
			role := "Participant"
			if len(tag) >= 4 {
				role = tag[3]
			}
			participants = append(participants, fmt.Sprintf("%s (%s)", tag[1][:8], role))
		}
	}

	// Build message
	var msg strings.Builder
	msg.WriteString("\n ===== \n🎯 **New Live Activity Update**\n\n")

	msg.WriteString(fmt.Sprintf("👤 **Author:** %s\n", authorNpub))

	if title != "" {
		msg.WriteString(fmt.Sprintf("📌 **Title:** %s\n", title))
	}
	if summary != "" {
		msg.WriteString(fmt.Sprintf("📝 **Summary:** %s\n", summary))
	}
	if status != "" {
		emoji := "🔄"
		switch status {
		case "planned":
			emoji = "📅"
		case "live":
			emoji = "🟢"
		case "ended":
			emoji = "🔴"
		}
		msg.WriteString(fmt.Sprintf("%s **Status:** %s\n", emoji, status))
	}
	if streaming != "" {
		msg.WriteString(fmt.Sprintf("🎥 **Stream:** %s\n", streaming))
	}
	if starts != "" {
		msg.WriteString(fmt.Sprintf("⏰ **Starts:** %s\n", starts))
	}
	if ends != "" {
		msg.WriteString(fmt.Sprintf("🏁 **Ends:** %s\n", ends))
	}
	if len(participants) > 0 {
		msg.WriteString(fmt.Sprintf("👥 **Participants:** %s\n", strings.Join(participants, ", ")))
	}
	if image != "" {
		msg.WriteString(fmt.Sprintf("\n%s", image))
	}

	return msg.String()
}

func prettyJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func sendToDiscord(webhookURL string, message DiscordWebhookMessage) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("discord webhook returned status: %d", resp.StatusCode)
	}

	return nil
}

func main() {
	log.Println("Starting Nostr event listener...")
	listenToNostrEvents()
}
