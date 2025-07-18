package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
)

// HiveTalk API response structure for Honey
type Room struct {
	Name            string     `json:"name"`
	Sid             string     `json:"sid"`
	CreatedAt       time.Time  `json:"createdAt"`
	NumParticipants int        `json:"numParticipants"`
	Description     *string    `json:"description,omitempty"`
	PictureUrl      *string    `json:"pictureUrl,omitempty"`
	Status          *string    `json:"status,omitempty"`
}

// Simple database to track rooms and their status
type RoomDatabase struct {
	Rooms map[string]RoomInfo
	Path  string
}

type RoomInfo struct {
	DTag      string    `json:"d_tag"`
	RoomName  string    `json:"room_name"`
	Status    string    `json:"status"`
	LastSeen  time.Time `json:"last_seen"`
}

// Global random source
var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

// Generate a unique d tag for a room
func generateDTag() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, 10)
	for i := range result {
		result[i] = charset[rnd.Intn(len(charset))]
	}
	return string(result)
}

// Load the room database from a file
func loadRoomDatabase(path string) (*RoomDatabase, error) {
	db := &RoomDatabase{
		Rooms: make(map[string]RoomInfo),
		Path:  path,
	}

	// Check if the file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Create a new file
		return db, db.save()
	}

	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Unmarshal the data
	if len(data) > 0 {
		if err := json.Unmarshal(data, &db.Rooms); err != nil {
			return nil, err
		}
	}

	return db, nil
}

// Save the room database to a file
func (db *RoomDatabase) save() error {
	data, err := json.MarshalIndent(db.Rooms, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(db.Path, data, 0644)
}

// Get the d tag for a room, creating one if it doesn't exist
func (db *RoomDatabase) getDTag(roomID string) string {
	if info, exists := db.Rooms[roomID]; exists {
		return info.DTag
	}

	// Create a new d tag
	dTag := generateDTag()
	db.Rooms[roomID] = RoomInfo{
		DTag:     dTag,
		RoomName: "Unknown Room", // Default room name
		Status:   "unknown",
		LastSeen: time.Time{},
	}
	if err := db.save(); err != nil {
		log.Printf("Error saving room database after creating dTag for room %s: %v", roomID, err)
	}
	return dTag
}

// Helper function to get room name from the database for use in discord.go
func getRoomInfoFromDatabase(roomID string) string {
	// Load the database
	db, err := loadRoomDatabase("honey_rooms.json")
	if err != nil {
		return ""
	}

	// Get the room name
	if info, exists := db.Rooms[roomID]; exists && info.RoomName != "" {
		return info.RoomName
	}

	return ""
}

// Update the status of a room
func (db *RoomDatabase) updateRoomStatus(roomID, roomName, status string) bool {
	info, exists := db.Rooms[roomID]
	if !exists {
		info = RoomInfo{
			DTag:     db.getDTag(roomID),
			RoomName: roomName,
			Status:   status,
			LastSeen: time.Now(),
		}
		db.Rooms[roomID] = info
		if err := db.save(); err != nil {
			log.Printf("Error saving room database after creating new room %s: %v", roomID, err)
		}
		return true // Status changed
	}

	if info.Status != status || info.RoomName != roomName {
		info.Status = status
		info.RoomName = roomName
		info.LastSeen = time.Now()
		db.Rooms[roomID] = info
		if err := db.save(); err != nil {
			log.Printf("Error saving room database after updating status for room %s: %v", roomID, err)
		}
		return true // Status or room name changed
	}

	// Update last seen time
	info.LastSeen = time.Now()
	db.Rooms[roomID] = info
	if err := db.save(); err != nil {
		log.Printf("Error saving room database after updating last seen time for room %s: %v", roomID, err)
	}
	return false // Status didn't change
}

// Check for rooms that have closed
func (db *RoomDatabase) checkClosedRooms(activeRoomIDs []string) []string {
	closedRooms := []string{}
	
	// Convert active room IDs to a map for faster lookup
	activeRoomMap := make(map[string]bool)
	for _, roomID := range activeRoomIDs {
		activeRoomMap[roomID] = true
	}

	// Check for rooms that were previously open but are not in the active list
	for roomID, info := range db.Rooms {
		if info.Status == "open" && !activeRoomMap[roomID] {
			// Room is no longer active
			closedRooms = append(closedRooms, roomID)
			// For closed rooms, use the stored room name if available, otherwise use "Closed Room"
			roomName := "Closed Room"
			if info, exists := db.Rooms[roomID]; exists && info.RoomName != "" {
				roomName = info.RoomName
			}
			if db.updateRoomStatus(roomID, roomName, "closed") {
				log.Printf("Room %s marked as closed", roomID)
			}
		}
	}

	return closedRooms
}

// Fetch rooms from the Honey API
func fetchRooms(baseURL string) ([]Room, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rooms []Room
	if err := json.Unmarshal(body, &rooms); err != nil {
		return nil, err
	}

	return rooms, nil
}

// Create and publish a 30312 event
func publishEvent(ctx context.Context, privateKey, roomID, roomName, dTag, status, summary, imageURL, serviceURL string, relayURLs []string) error {
	log.Printf("Publishing %s event for room %s with dTag %s", status, roomID, dTag)
	
	// Get public key from private key
	pubkey, err := nostr.GetPublicKey(privateKey)
	if err != nil {
		return fmt.Errorf("error getting public key: %v", err)
	}
	log.Printf("Using pubkey: %s", pubkey)

	// Create event tags
	tags := nostr.Tags{
		nostr.Tag{"d", dTag},
		nostr.Tag{"room", roomName}, // Use room name for the room tag
		nostr.Tag{"summary", summary},
		nostr.Tag{"status", status},
		nostr.Tag{"image", imageURL},
		nostr.Tag{"service", serviceURL},
	}

	// Add t tags
	tags = append(tags, nostr.Tag{"t", "hivetalk-honey"})
	tags = append(tags, nostr.Tag{"t", "interactive room"})

	// Add relays tag
	relaysTag := []string{"relays"}
	relaysTag = append(relaysTag, relayURLs...)
	tags = append(tags, relaysTag)

	// Create event
	ev := nostr.Event{
		PubKey:    pubkey,
		CreatedAt: nostr.Now(),
		Kind:      30312,
		Tags:      tags,
		Content:   "",
	}

	// Sign the event
	if err := ev.Sign(privateKey); err != nil {
		return fmt.Errorf("error signing event: %v", err)
	}
	log.Printf("Event signed with ID: %s", ev.ID)

	// Publish to each relay
	for _, url := range relayURLs {
		// Trim any whitespace
		url = strings.TrimSpace(url)
		log.Printf("Connecting to relay: %s", url)

		// Create a timeout context for each relay connection
		relayCtx, relayCancel := context.WithTimeout(ctx, 10*time.Second)

		relay, err := nostr.RelayConnect(relayCtx, url)
		if err != nil {
			log.Printf("Error connecting to relay %s: %v\n", url, err)
			relayCancel() // Cancel context if connection fails
			continue
		}

		publishStatus, err := relay.Publish(relayCtx, ev)

		// Always close the relay and cancel context when done
		relay.Close()
		relayCancel()

		if err != nil {
			log.Printf("Error publishing to %s: %v\n", url, err)
			continue
		}
		log.Printf("Published event for room %s with status %s to %s, relay status: %v\n", roomID, status, url, publishStatus)
	}

	return nil
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	log.Println("Environment variables loaded")

	// Get environment variables
	baseURL := os.Getenv("BASE_URL")
	privateKey := os.Getenv("NOSTR_PVT_KEY")
	relayURLsStr := os.Getenv("RELAY_URLS")
	discordURL := os.Getenv("DISCORD_URL")

	// Validate required environment variables
	if baseURL == "" {
		log.Fatalf("Missing BASE_URL environment variable. Please check your .env file.")
	}

	// Check if Nostr integration is enabled
	nostrEnabled := privateKey != "" && relayURLsStr != ""
	if !nostrEnabled {
		log.Println("Nostr integration disabled - missing NOSTR_PVT_KEY or RELAY_URLS")
	}

	// Log integration status
	log.Printf("Using base URL: %s", baseURL)

	if discordURL != "" {
		log.Printf("Discord integration enabled")
	}

	if nostrEnabled {
		log.Printf("Nostr integration enabled")
		log.Printf("Relay URLs: %s", relayURLsStr)
	}

	// Parse relay URLs if Nostr is enabled
	relayURLs := []string{}
	if nostrEnabled {
		for _, url := range strings.Split(relayURLsStr, ",") {
			url = strings.TrimSpace(url)
			if url != "" {
				relayURLs = append(relayURLs, url)
			}
		}

		if len(relayURLs) == 0 {
			log.Println("Warning: No valid relay URLs found. Nostr publishing will be disabled.")
			nostrEnabled = false
		} else {
			log.Printf("Found %d relay URLs", len(relayURLs))
		}
	}

	// Load or create the room database
	db, err := loadRoomDatabase("honey_rooms.json")
	if err != nil {
		log.Fatalf("Error loading room database: %v", err)
	}
	log.Printf("Room database loaded with %d rooms", len(db.Rooms))

	// Create context
	ctx := context.Background()

	// Polling interval (60 seconds)
	interval := 60 * time.Second

	log.Printf("Polling %s every %v", baseURL, interval)

	// Main polling loop
	for {
		log.Println("Polling for rooms...")
		
		// Fetch rooms
		rooms, err := fetchRooms(baseURL)
		if err != nil {
			log.Printf("Error fetching rooms: %v", err)
			time.Sleep(interval)
			continue
		}
		log.Printf("Found %d active rooms", len(rooms))

		activeRoomIDs := []string{}

		// Track status changes for Discord notifications
		statusChanges := make(map[string]string)

		// Process each room
		for _, room := range rooms {
			log.Printf("Processing room: %s - %s with %d participants", room.Sid, room.Name, room.NumParticipants)
			activeRoomIDs = append(activeRoomIDs, room.Sid)
			
			// Get or create d tag for this room
			dTag := db.getDTag(room.Sid)
			log.Printf("Using dTag %s for room %s", dTag, room.Sid)

			// Determine room status
			roomStatus := "open"
			// If status is explicitly set, use it
			if room.Status != nil {
				roomStatus = *room.Status
			} else if room.NumParticipants == 0 {
				// If no participants, treat as closed
				roomStatus = "closed"
				log.Printf("Room %s has 0 participants, marking as closed", room.Sid)
			}
			statusChanged := db.updateRoomStatus(room.Sid, room.Name, roomStatus)

			// Track status changes for Discord notifications
			if statusChanged {
				statusChanges[room.Sid] = roomStatus
			}

			// Publish event if status changed and Nostr is enabled
			if statusChanged {
				log.Printf("Room %s status changed to %s", room.Sid, roomStatus)

				// Construct service URL using room name
				serviceURL := fmt.Sprintf("https://honey.hivetalk.org/meet/%s", url.PathEscape(room.Name))

				// Use description for summary tag and name for room tag
				// Default summary to room name if description is nil
				summary := room.Name
				imageURL := "https://honey.hivetalk.org/logo.png"
				if room.Description != nil {
					summary = *room.Description
				}
				if room.PictureUrl != nil {
					imageURL = *room.PictureUrl
				}

				// publish everything both ephemeral and permanent rooms to all relays for rebroadcast
				log.Printf("Publishing event for room %s", room.Sid)
				if err := publishEvent(ctx, privateKey, room.Sid, room.Name, dTag, roomStatus, summary, imageURL, serviceURL, relayURLs); err != nil {
					log.Printf("Error publishing event for room %s: %v", room.Sid, err)
				}

				// Only publish to Nostr if enabled AND the room doesn't already have a status field with a value
				// If the room has a status field with a value, it means the data is already being published elsewhere
				// hasStatus := room.Status != nil && *room.Status != ""
				// if nostrEnabled && !hasStatus {
				// 	log.Printf("Publishing event for room %s", room.Sid)
				// 	if err := publishEvent(ctx, privateKey, room.Sid, room.Name, dTag, roomStatus, summary, imageURL, serviceURL, relayURLs); err != nil {
				// 		log.Printf("Error publishing event for room %s: %v", room.Sid, err)
				// 	}
				// } else if room.Status != nil && *room.Status != "" {
				// 	log.Printf("Skipping Nostr publishing for room %s as it already has a status field: %s", room.Sid, *room.Status)
				// }
			} else {
				log.Printf("Room %s already %s, no event published", room.Sid, roomStatus)
			}
		}

		// Check for rooms that are no longer in the API response
		closedRooms := db.checkClosedRooms(activeRoomIDs)
		log.Printf("Found %d closed rooms", len(closedRooms))
		for _, roomID := range closedRooms {
			dTag := db.getDTag(roomID)
			log.Printf("Room %s closed, publishing closed event with dTag %s", roomID, dTag)

			// Track status changes for Discord notifications
			statusChanges[roomID] = "closed"

			// For closed rooms, get the stored room name from the database
			roomName := "Unknown Room"
			if info, exists := db.Rooms[roomID]; exists && info.RoomName != "" {
				roomName = info.RoomName
			}

			// Use the actual room name for the event
			serviceURL := fmt.Sprintf("https://honey.hivetalk.org/meet/%s", url.PathEscape(roomName))
			summary := fmt.Sprintf("%s is now closed", roomName)
			
			// Only publish to Nostr if enabled
			if nostrEnabled {
				log.Printf("Publishing closed event for room %s", roomID)
				if err := publishEvent(ctx, privateKey, roomID, roomName, dTag, "closed", summary, "", serviceURL, relayURLs); err != nil {
					log.Printf("Error publishing closed event for room %s: %v", roomID, err)
				}
			}
		}

		// Send updates to Discord if enabled
		if discordURL != "" && len(statusChanges) > 0 {
			log.Printf("Sending %d room updates to Discord", len(statusChanges))
			SendRoomUpdatesToDiscord(ctx, discordURL, rooms, statusChanges)
		}

		log.Printf("Sleeping for %v before next poll", interval)
		// Wait for the next polling interval
		time.Sleep(interval)
	}
}
