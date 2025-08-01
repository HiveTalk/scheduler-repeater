package main

import (
	"bufio"
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
	"golang.org/x/time/rate"
)

type DiscordWebhookMessage struct {
	Content string `json:"content"`
}

// Global rate limiter for Discord webhooks
var discordLimiter = rate.NewLimiter(rate.Every(time.Second/5), 1) // 5 requests per second max

// Maximum Discord message size
const maxDiscordMessageSize = 2000

// loadEnv loads environment variables from .env file
func loadEnv() {
	envFile := ".env"
	file, err := os.Open(envFile)
	if err != nil {
		// If .env file doesn't exist, just return without error
		if os.IsNotExist(err) {
			return
		}
		log.Printf("Warning: Error opening .env file: %v", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip empty lines and comments
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE format
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if len(value) > 1 && (strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) ||
			(strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) {
			value = value[1 : len(value)-1]
		}

		// Only set if not already in environment
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Warning: Error reading .env file: %v", err)
	}
}

func truncateMessage(message string, maxSize int) string {
	if len(message) <= maxSize {
		return message
	}
	// Keep some room for the truncation notice
	return message[:maxSize-50] + "\n... [message truncated due to Discord size limits]"
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

		// Calculate the timestamp for 7 days ago
		sevenDaysAgo := time.Now().AddDate(0, 0, -7).Unix()
		timestamp := nostr.Timestamp(sevenDaysAgo)
		// Subscribe to kind 30311, 30312, and 30313 events (NIP-53 Live Activities)
		// timestamp := nostr.Timestamp(time.Now().Unix())
		sub, err := relay.Subscribe(ctx, []nostr.Filter{{
			Kinds: []int{30311, 30312, 30313},
			Since: &timestamp, // Pass the address of the timestamp
		}})
		if err != nil {
			log.Printf("Failed to subscribe: %v. Retrying in 5 seconds...", err)
			cancel()
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("Connected to relay %s and subscribed to NIP-53 Live Activity events (kind 30311, 30312, 30313)", relayURL)

		// Listen for events
		for event := range sub.Events {
			log.Printf("Received NIP-53 event with ID: %s, Kind: %d", event.ID, event.Kind)

			// Format the message first
			formattedMsg := formatNostrMessage(event, nil)
			// Check if adding the full JSON would exceed the limit
			// omit jsonPart for now
			//jsonPart := "\n\n**Original Event JSON:**\n```json\n" + prettyJSON(event) + "\n```"
			fullMsg := formattedMsg //+ jsonPart
			// If the full message is too long, truncate the JSON part or omit it
			if len(fullMsg) > maxDiscordMessageSize {
				if len(formattedMsg) > maxDiscordMessageSize {
					// Even the formatted message is too long
					formattedMsg = truncateMessage(formattedMsg, maxDiscordMessageSize)
					fullMsg = formattedMsg
				} else {
					// Try to include a truncated JSON
					remaining := maxDiscordMessageSize - len(formattedMsg) - 50 // 50 chars for wrapper and truncation notice
					if remaining > 100 { // Only include JSON if we have reasonable space
						truncatedJSON := prettyJSON(event)
						if len(truncatedJSON) > remaining {
							truncatedJSON = truncatedJSON[:remaining] + "...\n[truncated]"
						}
						fullMsg = formattedMsg + "\n\n**Original Event JSON (truncated):**\n```json\n" + truncatedJSON + "\n```"
					} else {
						// Not enough space for JSON
						fullMsg = formattedMsg + "\n\n*Event JSON omitted due to size constraints*"
					}
				}
			}

			// Create Discord message
			message := DiscordWebhookMessage{
				Content: fullMsg,
			}

			// Wait for rate limiter before sending
			if err := discordLimiter.Wait(ctx); err != nil {
				log.Printf("Rate limiter error: %v", err)
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
	var title, summary, image, status, starts, ends, streaming, service, room string
	var participants []string

	// Convert pubkey to npub
	npub, _ := nip19.EncodePublicKey(event.PubKey)
	authorNpub := npub[:8] + "..." // Take first 8 chars

	// Determine kind description
	kindDescription := "Unknown"
	switch event.Kind {
	case 30311:
		kindDescription = "Live Activities"
	case 30312:
		kindDescription = "Interactive Rooms"
	case 30313:
		kindDescription = "Scheduled Meeting Room"
	}

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
		case "service":
			service = tag[1]
		case "room":
			room = tag[1]
		case "p":
			role := "owner"
			if len(tag) >= 4 {
				role = tag[3]
			}
			npub, _ := nip19.EncodePublicKey(tag[1])
			njump := "https://njump.me/" + npub
			participants = append(participants, fmt.Sprintf("%s (%s)", njump, role))
			//participants = append(participants, fmt.Sprintf("%s (%s)", tag[1][:8], role))
		}
	}

	// Build message
	var msg strings.Builder
	msg.WriteString("\n ===== 🎯 **New Nostr Event Update** ======\n\n")

	msg.WriteString(fmt.Sprintf("👤 **Author:** %s\n", authorNpub))
	msg.WriteString(fmt.Sprintf("🔢 **Kind:** %d - %s\n", event.Kind, kindDescription))

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
	if service != "" {
		msg.WriteString(fmt.Sprintf("🔗 **Service:** %s\n", service))
	}
	if room != "" {
		msg.WriteString(fmt.Sprintf("🏠 **Room:** %s\n", room))
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
	// Load environment variables from .env file
	loadEnv()
	listenToNostrEvents()
}
