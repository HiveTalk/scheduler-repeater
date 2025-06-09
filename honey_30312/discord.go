package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

// DiscordWebhookMessage represents the structure of a Discord webhook message
type DiscordWebhookMessage struct {
	Content string `json:"content"`
}

// Global rate limiter for Discord webhooks
// 5 requests per second max with a burst of 1
var discordLimiter = rate.NewLimiter(rate.Every(time.Second/5), 1)

// Maximum Discord message size
const maxDiscordMessageSize = 2000

// Maximum number of rooms to include in a single Discord message
const maxRoomsPerMessage = 2

// truncateMessage truncates a message to fit within Discord's message size limits
func truncateMessage(message string, maxSize int) string {
	if len(message) <= maxSize {
		return message
	}
	// Keep some room for the truncation notice
	return message[:maxSize-50] + "\n... [message truncated due to Discord size limits]"
}

// sendToDiscord sends a message to a Discord webhook
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

// formatRoomMessage formats a room update message for Discord
func formatRoomMessage(room Room, status string) string {
	var msg string
	
	// Determine emoji based on status
	emoji := "🔄"
	switch status {
	case "open":
		emoji = "🟢"
	case "closed":
		emoji = "🔴"
	}
	
	// Format the basic room info
	msg = fmt.Sprintf("%s **Room Update: %s**\n", emoji, room.Name)
	msg += fmt.Sprintf("**Status:** %s\n", status)
	msg += fmt.Sprintf("**Room ID:** %s\n", room.Sid)
	msg += fmt.Sprintf("**Participants:** %d\n", room.NumParticipants)
	
	// Add description if available
	if room.Description != nil {
		msg += fmt.Sprintf("**Description:** %s\n", *room.Description)
	}
	
	// Add created time
	msg += fmt.Sprintf("**Created At:** %s\n", room.CreatedAt.Format(time.RFC1123))
	
	// Add service URL using room name
	msg += fmt.Sprintf("**Join URL:** https://honey.hivetalk.org/meet/%s\n", url.PathEscape(room.Name))
	
	// Add separator
	msg += "----------------------------\n"
	
	return msg
}

// SendRoomUpdatesToDiscord sends room updates to Discord
// It handles batching messages to avoid Discord rate limits
func SendRoomUpdatesToDiscord(ctx context.Context, webhookURL string, rooms []Room, statusChanges map[string]string) {
	if webhookURL == "" {
		// Discord webhook URL not provided, skip
		return
	}
	
	if len(rooms) == 0 && len(statusChanges) == 0 {
		// No updates to send
		return
	}
	
	log.Printf("Sending %d room updates to Discord", len(statusChanges))
	
	// Group rooms by status for better organization
	openRooms := []Room{}
	closedRooms := []Room{}
	
	// Find rooms with status changes
	for _, room := range rooms {
		if newStatus, ok := statusChanges[room.Sid]; ok {
			if newStatus == "open" {
				openRooms = append(openRooms, room)
			} else if newStatus == "closed" {
				closedRooms = append(closedRooms, room)
			}
		}
	}
	
	// Add closed rooms that are no longer in the API response
	for roomID, status := range statusChanges {
		if status == "closed" {
			// Check if this room is already in closedRooms
			found := false
			for _, room := range closedRooms {
				if room.Sid == roomID {
					found = true
					break
				}
			}
			
			if !found {
				// Get the room name from the database for closed rooms
				roomName := "Unknown Room"
				
				// Try to get the room name from the database
				// We'll access the database directly
				if roomInfo := getRoomInfoFromDatabase(roomID); roomInfo != "" {
					roomName = roomInfo
				}
				
				// Create a placeholder room with the correct name
				closedRooms = append(closedRooms, Room{
					Name:            roomName,
					Sid:             roomID,
					CreatedAt:       time.Now(),
					NumParticipants: 0,
				})
			}
		}
	}
	
	// Send open room updates
	if len(openRooms) > 0 {
		sendRoomBatch(ctx, webhookURL, openRooms, "open")
	}
	
	// Send closed room updates
	if len(closedRooms) > 0 {
		sendRoomBatch(ctx, webhookURL, closedRooms, "closed")
	}
}

// sendRoomBatch sends a batch of room updates to Discord
// It splits messages if there are too many rooms to fit in one message
func sendRoomBatch(ctx context.Context, webhookURL string, rooms []Room, status string) {
	// Create batches of rooms
	var batches [][]Room
	for i := 0; i < len(rooms); i += maxRoomsPerMessage {
		end := i + maxRoomsPerMessage
		if end > len(rooms) {
			end = len(rooms)
		}
		batches = append(batches, rooms[i:end])
	}
	
	// Send each batch
	for _, batch := range batches {
		var message string
		
		message = ""
		// Add header based on status
		// if status == "open" {
		// 	message = "🟢 New Open Rooms\n\n"
		// } else {
		// 	message = "🔴 Recently Closed Rooms\n\n"
		// }
		
		// Add each room to the message
		for _, room := range batch {
			message += formatRoomMessage(room, status)
		}
		
		// Truncate if necessary
		if len(message) > maxDiscordMessageSize {
			message = truncateMessage(message, maxDiscordMessageSize)
		}
		
		// Create Discord message
		discordMsg := DiscordWebhookMessage{
			Content: message,
		}
		
		// Wait for rate limiter
		if err := discordLimiter.Wait(ctx); err != nil {
			log.Printf("Error waiting for rate limiter: %v", err)
			continue
		}
		
		// Send to Discord with retries
		for retries := 0; retries < 3; retries++ {
			if err := sendToDiscord(webhookURL, discordMsg); err != nil {
				if retries < 2 {
					log.Printf("Failed to send to Discord: %v. Retrying in 2 seconds...", err)
					time.Sleep(2 * time.Second)
					continue
				}
				log.Printf("Failed to send to Discord after 3 attempts: %v", err)
			} else {
				log.Printf("Successfully sent Discord message for %d rooms with status %s", len(batch), status)
				break
			}
		}
		
		// Add delay between batches to avoid rate limiting
		time.Sleep(1 * time.Second)
	}
}
