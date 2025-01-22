package events

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

type EventData struct {
	ProfileID    string    `json:"profile_id"`
	NaddrID     string    `json:"naddr_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ImageURL    string    `json:"image_url"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	IsPaidEvent bool      `json:"is_paid_event"`
	RoomName    string    `json:"room_name"`
	Identifier  string    `json:"identifier"`
	NostrPubkey string    `json:"nostr_pubkey"`
}

type Nip53Event struct {
	Kind      int       `json:"kind"`
	CreatedAt int64     `json:"created_at"`
	Pubkey    string    `json:"pubkey"`
	Tags      [][]string `json:"tags"`
	Content   string    `json:"content"`
}

func generateNip53(event EventData, pubkey string, hiveURL string) *Nip53Event {
	startTime := event.StartTime.Unix()
	endTime := event.EndTime.Unix()
	status := "planned" // planned, live, ended
	roomName := event.RoomName

	// if room name is random room, then use the event id for the hive room
	if roomName == "Random Room" {
		roomName = event.ProfileID
	}

	return &Nip53Event{
		Kind:      30311,
		CreatedAt: time.Now().Unix(),
		Pubkey:    pubkey,
		Tags: [][]string{
			{"d", event.Identifier},
			{"title", event.Name},
			{"starts", fmt.Sprintf("%d", startTime)},
			{"ends", fmt.Sprintf("%d", endTime)},
			{"streaming", fmt.Sprintf("%s/join/%s", hiveURL, roomName)},
			{"summary", event.Description},
			{"image", event.ImageURL},
			{"status", status},
			{"t", "nostr"},
			{"t", "hivetalk"},
			{"t", "livestream"},
		},
		Content: "",
	}
}

func ProcessNostrEvent(event EventData, hiveURL string, privateKey string, nostrClient *NostrClient) error {
	// Only process if we have an identifier (meaning it's an update)
	if event.Identifier == "" {
		return fmt.Errorf("no identifier found, skipping event")
	}

	// Generate NIP-53 event for update
	nip53Event := generateNip53(event, event.NostrPubkey, hiveURL)

	// Sign the event
	signedEvent, err := SignEvent(nip53Event, privateKey)
	if err != nil {
		return fmt.Errorf("failed to sign event: %v", err)
	}

	// Send event to relays
	if err := SendToRelays(signedEvent, nostrClient); err != nil {
		return fmt.Errorf("failed to send event to relays: %v", err)
	}

	return nil
}

// ProcessLatestEvents is the main function to be called by the scheduler
func ProcessLatestEvents(db *Database, nostrClient *NostrClient) error {
	hiveURL := os.Getenv("HIVETALK_URL")
	if hiveURL == "" {
		return fmt.Errorf("HIVETALK_URL environment variable not set")
	}

	privateKey := os.Getenv("NOSTR_PRIVATE_KEY")
	if privateKey == "" {
		return fmt.Errorf("NOSTR_PRIVATE_KEY environment variable not set")
	}

	// Fetch only events that need updating (have nostr_status = 'pending')
	events, err := db.GetUnprocessedEvents()
	if err != nil {
		return fmt.Errorf("failed to fetch events pending update: %v", err)
	}

	for _, event := range events {
		// Skip if no identifier (not an update)
		if event.Identifier == "" {
			continue
		}

		err := ProcessNostrEvent(event, hiveURL, privateKey, nostrClient)
		if err != nil {
			log.Printf("Error processing event update %s: %v", event.Identifier, err)
			// Mark event as failed but continue processing others
			if dbErr := db.MarkEventAsFailed(event.Identifier, err.Error()); dbErr != nil {
				log.Printf("Error marking event as failed: %v", dbErr)
			}
			continue
		}

		// Mark event as processed
		if err := db.MarkEventAsProcessed(event.Identifier); err != nil {
			log.Printf("Error marking event %s as processed: %v", event.Identifier, err)
		}
	}

	return nil
}
