package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

var relayurl = getRelayUrl()
var defaultRelays = []string{relayurl}
var hivetalkURL = getRequiredEnv("HIVETALK_URL")

func getRelayUrl() string {
	relayURL := os.Getenv("RELAY_URL")
	if relayURL == "" {
		log.Fatal("RELAY_URL environment variable is required")
	}
}

func getRequiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Environment variable %s is required but not set", key)
	}
	return value
}

type Event struct {
	ID          string    `json:"id"` // Changed from int to string to handle UUID
	Name        string    `json:"name"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	RoomName    *string   `json:"room_name"`   // Nullable
	Identifier  *string   `json:"identifier"`  // Nullable
	Description *string   `json:"description"` // Nullable
	Image       *string   `json:"image"`       // Nullable
	Status      *string   `json:"status"`      // Nullable
}

// Helper functions for Event struct
func (e *Event) GetIdentifier() string {
	if e.Identifier == nil {
		return ""
	}
	return *e.Identifier
}

func (e *Event) GetRoomName() string {
	if e.RoomName == nil {
		return ""
	}
	return *e.RoomName
}

func (e *Event) GetDescription() string {
	if e.Description == nil {
		return ""
	}
	return *e.Description
}

func (e *Event) GetImage() string {
	if e.Image == nil {
		return ""
	}
	return *e.Image
}

func (e *Event) GetStatus() string {
	if e.Status == nil {
		return ""
	}
	return *e.Status
}

type RoomInfo struct {
	RoomNpub     *string `json:"room_npub"`
	RoomNsec     *string `json:"room_nsec"`
	RoomRelayURL *string `json:"room_relay_url"`
}

func getSupabaseConnection() (*pgxpool.Pool, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable not set")
	}

	// Parse the connection string to modify it
	connConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %v", err)
	}

	// Add SSL mode and other required settings
	if connConfig.ConnConfig.TLSConfig == nil {
		connConfig.ConnConfig.TLSConfig = &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		}
	}

	// Disable prepared statement cache to avoid conflicts
	connConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	// Set pool configuration
	connConfig.MaxConns = 4
	connConfig.MinConns = 1
	connConfig.MaxConnLifetime = time.Hour
	connConfig.MaxConnIdleTime = 30 * time.Minute

	// Set shorter timeout for pooled connections
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create the connection pool
	pool, err := pgxpool.NewWithConfig(ctx, connConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %v", err)
	}

	// Test the pool
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connection pool test failed: %v", err)
	}

	return pool, nil
}

func updateNip53(eventData Event, pubkey string, status string) (*nostr.Event, error) {
	startTime := eventData.StartTime.Unix()
	endTime := eventData.EndTime.Unix()

	tags := nostr.Tags{
		{"d", eventData.GetIdentifier()},
		{"title", eventData.Name},
		{"starts", fmt.Sprintf("%d", startTime)},
		{"ends", fmt.Sprintf("%d", endTime)},
		{"status", status},
		{"streaming", hivetalkURL + "/join/" + eventData.GetRoomName()},
		{"t", "nostr"},
		{"t", "hivetalk"},
		{"t", "livestream"},
	}

	if eventData.GetDescription() != "" {
		tags = append(tags, nostr.Tag{"description", eventData.GetDescription()})
	}
	if eventData.GetImage() != "" {
		tags = append(tags, nostr.Tag{"image", eventData.GetImage()})
	}

	event := &nostr.Event{
		PubKey:    pubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      30311,
		Tags:      tags,
		Content:   "",
	}

	return event, nil
}

func sendLiveEvent(event *nostr.Event, relays []string) error {
	ctx := context.Background()

	for _, url := range relays {
		relay, err := nostr.RelayConnect(ctx, url)
		if err != nil {
			log.Printf("Failed to connect to relay %s: %v", url, err)
			continue
		}

		// Create a timeout context for publishing
		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)

		err = relay.Publish(publishCtx, *event)
		if err != nil {
			log.Printf("Failed to publish to relay %s: %v", url, err)
			cancel()
			relay.Close()
			continue
		}

		cancel()
		relay.Close()
	}
	return nil
}

func sendNewEvent(payload Event, status string) error {
	conn, err := getSupabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer conn.Close()

	var roomInfo RoomInfo
	err = conn.QueryRow(context.Background(),
		"SELECT room_npub, room_nsec, room_relay_url FROM room_info WHERE room_name = $1",
		payload.RoomName).Scan(&roomInfo.RoomNpub, &roomInfo.RoomNsec, &roomInfo.RoomRelayURL)
	if err != nil {
		return fmt.Errorf("failed to fetch room info: %v", err)
	}

	// Check if required fields are present
	if roomInfo.RoomNsec == nil {
		return fmt.Errorf("room_nsec is required but not set for room %s", payload.RoomName)
	}

	relays := defaultRelays
	if roomInfo.RoomRelayURL != nil && *roomInfo.RoomRelayURL != "" {
		relays = append(relays, *roomInfo.RoomRelayURL)
	}

	// Decode the private key from nsec
	prefix, privKey, err := nip19.Decode(*roomInfo.RoomNsec)
	if err != nil || prefix != "nsec" {
		return fmt.Errorf("failed to decode nsec or invalid prefix: %v", err)
	}
	sk := privKey.(string)

	// Get public key from private key
	pk, _ := nostr.GetPublicKey(sk)

	event, err := updateNip53(payload, pk, status)
	if err != nil {
		return fmt.Errorf("failed to create event: %v", err)
	}

	// Sign the event with private key
	event.Sign(sk)

	// Update the event ID in the database
	_, err = conn.Exec(context.Background(),
		"UPDATE events SET nostr_event_id = $1, status = $2 WHERE id = $3",
		event.ID, status+":sent", payload.ID)
	if err != nil {
		return fmt.Errorf("failed to update event in database: %v", err)
	}

	err = sendLiveEvent(event, relays)
	if err != nil {
		// Update status to failed
		_, updateErr := conn.Exec(context.Background(),
			"UPDATE events SET status = $1 WHERE id = $2",
			status+":failed", payload.ID)
		if updateErr != nil {
			log.Printf("Failed to update event status to failed: %v", updateErr)
		}
		return fmt.Errorf("failed to send event: %v", err)
	}

	return nil
}
