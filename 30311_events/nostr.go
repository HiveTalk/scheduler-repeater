package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

var defaultRelays = []string{
	"wss://relay.damus.io",
	"wss://relay.snort.social",
	"wss://nostr.wine",
	// Add more default relays as needed
}

type NostrClient struct {
	relays []string
	pool   *nostr.SimplePool
}

func NewNostrClient(relays []string) *NostrClient {
	if len(relays) == 0 {
		relays = defaultRelays
	}
	return &NostrClient{
		relays: relays,
		pool:   nostr.NewSimplePool(context.Background()),
	}
}

// SignEvent signs a Nostr event using the provided private key
func SignEvent(event *Nip53Event, privateKeyHex string) (*nostr.Event, error) {
	// Convert our custom event type to nostr.Event
	nostrEvent := &nostr.Event{
		PubKey:    event.Pubkey,
		CreatedAt: time.Unix(event.CreatedAt, 0),
		Kind:      event.Kind,
		Tags:      event.Tags,
		Content:   event.Content,
	}

	// Generate a random 32-byte ID
	id := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("failed to generate event ID: %v", err)
	}
	nostrEvent.ID = hex.EncodeToString(id)

	// Sign the event
	err := nostrEvent.Sign(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to sign event: %v", err)
	}

	return nostrEvent, nil
}

// SendToRelays sends a signed event to multiple relays
func SendToRelays(event *nostr.Event, client *NostrClient) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(client.relays))
	successChan := make(chan string, len(client.relays))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Send to all relays concurrently
	for _, url := range client.relays {
		wg.Add(1)
		go func(relayURL string) {
			defer wg.Done()

			// Publish event to relay
			status := client.pool.Submit(ctx, []string{relayURL}, event)
			if status != nostr.PublishStatusSent {
				errChan <- fmt.Errorf("failed to publish to %s: %v", relayURL, status)
				return
			}
			successChan <- relayURL
		}(url)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(errChan)
		close(successChan)
	}()

	// Collect results
	var errors []error
	var successes []string
	for err := range errChan {
		errors = append(errors, err)
	}
	for success := range successChan {
		successes = append(successes, success)
	}

	// If we have at least one successful relay, consider it a success
	if len(successes) > 0 {
		return nil
	}

	// If all relays failed, return an error
	if len(errors) > 0 {
		return fmt.Errorf("failed to publish to all relays: %v", errors)
	}

	return nil
}
