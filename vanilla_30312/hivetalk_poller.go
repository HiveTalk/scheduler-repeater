package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
)

// HiveTalk API response structures
type HiveTalkResponse struct {
	Meetings []Meeting `json:"meetings"`
}

type Meeting struct {
	RoomID string `json:"roomId"`
	Peers  []Peer `json:"peers"`
}

type Peer struct {
	Name      string  `json:"name"`
	Presenter bool    `json:"presenter"`
	Npub      *string `json:"npub"`
	Pubkey    *string `json:"pubkey"`
	Lnaddress *string `json:"lnaddress"`
}

// Simple database to track rooms and their status
type RoomDatabase struct {
	Rooms map[string]RoomInfo
	Path  string
}

type RoomInfo struct {
	DTag      string    `json:"d_tag"`
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
		Status:   "unknown",
		LastSeen: time.Time{},
	}
	if err := db.save(); err != nil {
		log.Printf("Error saving room database after creating dTag for room %s: %v", roomID, err)
	}
	return dTag
}

// Update the status of a room
func (db *RoomDatabase) updateRoomStatus(roomID, status string) bool {
	info, exists := db.Rooms[roomID]
	if !exists {
		info = RoomInfo{
			DTag:     db.getDTag(roomID),
			Status:   status,
			LastSeen: time.Now(),
		}
		db.Rooms[roomID] = info
		if err := db.save(); err != nil {
			log.Printf("Error saving room database after creating new room %s: %v", roomID, err)
		}
		return true // Status changed
	}

	if info.Status != status {
		info.Status = status
		info.LastSeen = time.Now()
		db.Rooms[roomID] = info
		if err := db.save(); err != nil {
			log.Printf("Error saving room database after updating status for room %s: %v", roomID, err)
		}
		return true // Status changed
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
			db.updateRoomStatus(roomID, "closed")
		}
	}

	return closedRooms
}

// Fetch meetings from the HiveTalk API
func fetchMeetings(baseURL, apiKey string) (*HiveTalkResponse, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", baseURL+"/api/v1/meetings", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("accept", "application/json")
	req.Header.Add("authorization", apiKey)

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

	var response HiveTalkResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// Create and publish a 30312 event
func publishEvent(ctx context.Context, privateKey, roomID, dTag, status string, ownerPubkey string, relayURLs []string) error {
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
		nostr.Tag{"room", roomID},
		nostr.Tag{"summary", "HiveTalk Room"},
		nostr.Tag{"status", status},
		nostr.Tag{"image", "https://hivetalk.org/logo.png"},
		nostr.Tag{"service", fmt.Sprintf("https://hivetalk.org/join/%s", roomID)},
	}

	// Add owner tag if available
	if ownerPubkey != "" {
		log.Printf("Adding owner pubkey: %s", ownerPubkey)
		tags = append(tags, nostr.Tag{"p", ownerPubkey, "owner"})
	}

	// Add t tags
	tags = append(tags, nostr.Tag{"t", "hivetalk"})
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

		relay, err := nostr.RelayConnect(ctx, url)
		if err != nil {
			log.Printf("Error connecting to relay %s: %v\n", url, err)
			continue
		}
		defer relay.Close()

		publishStatus, err := relay.Publish(ctx, ev)
		if err != nil {
			log.Printf("Error publishing to %s: %v\n", url, err)
			continue
		}
		log.Printf("Published event for room %s with status %s to %s, relay status: %v\n", roomID, status, url, publishStatus)
	}

	return nil
}

func main() {
	// Configure logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting HiveTalk poller...")
	
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	log.Println("Environment variables loaded")

	// Get environment variables
	baseURL := os.Getenv("BASE_URL")
	apiKey := os.Getenv("HIVETALK_API_KEY")
	privateKey := os.Getenv("NOSTR_PVT_KEY")
	relayURLsStr := os.Getenv("RELAY_URLS")

	// Validate environment variables
	if baseURL == "" || apiKey == "" || privateKey == "" || relayURLsStr == "" {
		log.Fatalf("Missing required environment variables. Please check your .env file.")
	}
	log.Printf("Using base URL: %s", baseURL)
	log.Printf("Relay URLs: %s", relayURLsStr)

	// Parse relay URLs
	relayURLs := []string{}
	for _, url := range strings.Split(relayURLsStr, ",") {
		url = strings.TrimSpace(url)
		if url != "" {
			relayURLs = append(relayURLs, url)
		}
	}

	if len(relayURLs) == 0 {
		log.Fatalf("No relay URLs found. Please check your RELAY_URLS environment variable.")
	}
	log.Printf("Found %d relay URLs", len(relayURLs))

	// Load or create the room database
	db, err := loadRoomDatabase("rooms.json")
	if err != nil {
		log.Fatalf("Error loading room database: %v", err)
	}
	log.Printf("Room database loaded with %d rooms", len(db.Rooms))

	// Create context
	ctx := context.Background()

	// Polling interval (1 minute)
	interval := 1 * time.Minute

	log.Printf("Polling %s every %v", baseURL, interval)

	// Main polling loop
	for {
		log.Println("Polling for meetings...")
		
		// Fetch meetings
		response, err := fetchMeetings(baseURL, apiKey)
		if err != nil {
			log.Printf("Error fetching meetings: %v", err)
			time.Sleep(interval)
			continue
		}
		log.Printf("Found %d active meetings", len(response.Meetings))

		activeRoomIDs := []string{}

		// Process each meeting
		for _, meeting := range response.Meetings {
			log.Printf("Processing room: %s with %d peers", meeting.RoomID, len(meeting.Peers))
			activeRoomIDs = append(activeRoomIDs, meeting.RoomID)
			
			// Find the presenter (owner)
			var ownerPubkey string
			for _, peer := range meeting.Peers {
				if peer.Presenter && peer.Pubkey != nil {
					ownerPubkey = *peer.Pubkey
					log.Printf("Found presenter with pubkey: %s", ownerPubkey)
					break
				}
			}

			// Skip if no presenter with a pubkey
			if ownerPubkey == "" {
				log.Printf("Skipping room %s: No presenter with pubkey found", meeting.RoomID)
				continue
			}

			// Get or create d tag for this room
			dTag := db.getDTag(meeting.RoomID)
			log.Printf("Using dTag %s for room %s", dTag, meeting.RoomID)

			// Update room status
			statusChanged := db.updateRoomStatus(meeting.RoomID, "open")

			// Publish event if status changed
			if statusChanged {
				log.Printf("Room %s status changed to open, publishing event", meeting.RoomID)
				if err := publishEvent(ctx, privateKey, meeting.RoomID, dTag, "open", ownerPubkey, relayURLs); err != nil {
					log.Printf("Error publishing open event for room %s: %v", meeting.RoomID, err)
				}
			} else {
				log.Printf("Room %s already open, no event published", meeting.RoomID)
			}
		}

		// Check for closed rooms
		closedRooms := db.checkClosedRooms(activeRoomIDs)
		log.Printf("Found %d closed rooms", len(closedRooms))
		for _, roomID := range closedRooms {
			dTag := db.getDTag(roomID)
			log.Printf("Room %s closed, publishing closed event with dTag %s", roomID, dTag)
			if err := publishEvent(ctx, privateKey, roomID, dTag, "closed", "", relayURLs); err != nil {
				log.Printf("Error publishing closed event for room %s: %v", roomID, err)
			}
		}

		log.Printf("Sleeping for %v before next poll", interval)
		// Wait for the next polling interval
		time.Sleep(interval)
	}
}
