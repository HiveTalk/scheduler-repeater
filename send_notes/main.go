package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
)

const (
	batchSize    = 25 // Process up to 25 notes at a time
	maxWorkers   = 5  // Maximum number of concurrent workers
	pollInterval = 60 // Poll database every 60 seconds
)

// ScheduledNote represents a row from the scheduled_notes table
type ScheduledNote struct {
	ID            string     `json:"id"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ProfileID     string     `json:"profile_id"`
	Content       string     `json:"content"`
	ScheduledFor  time.Time  `json:"scheduled_for"`
	PublishedAt   *time.Time `json:"published_at"`
	Status        string     `json:"status"`
	RelayURLs     []string   `json:"relay_urls"`
	EventID       *string    `json:"event_id"`
	ErrorMessage  *string    `json:"error_message"`
	Signature     *string    `json:"signature"`
	SignedEvent   *string    `json:"signed_event"`
}

func init() {
	// Set up logging to file
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("Failed to create log directory: %v", err)
	}

	logFile := filepath.Join(logDir, fmt.Sprintf("send_notes_%s.log", time.Now().Format("2006-01-02")))
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}

	log.SetOutput(f)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting send_notes service...")

	// Load .env file
	err = godotenv.Load()
	if err != nil {
		log.Printf("Warning: Failed to load .env file: %v", err)
	}
}

func main() {
	ctx := context.Background()

	// Create database connection pool
	pool, err := getDBConnection(ctx)
	if err != nil {
		log.Fatalf("Failed to create database connection pool: %v", err)
	}
	defer pool.Close()

	log.Println("Database connection established")

	// Run the service in an infinite loop with periodic polling
	for {
		if err := processPendingNotes(ctx, pool); err != nil {
			log.Printf("Error processing pending notes: %v", err)
		}

		log.Printf("Sleeping for %d seconds before next poll", pollInterval)
		time.Sleep(time.Duration(pollInterval) * time.Second)
	}
}

func processPendingNotes(ctx context.Context, pool *pgxpool.Pool) error {
	// Get current time
	now := time.Now().UTC()
	log.Printf("Checking for scheduled notes to send at %v", now.Format(time.RFC3339))

	// Query for pending notes that are scheduled for now or earlier
	query := `
		SELECT 
			id, created_at, updated_at, profile_id, content, 
			scheduled_for, published_at, status, relay_urls, 
			event_id, error_message, signature, signed_event
		FROM scheduled_notes
		WHERE status = 'pending' 
		AND scheduled_for <= $1
		ORDER BY scheduled_for ASC
		LIMIT $2
	`

	rows, err := pool.Query(ctx, query, now, batchSize)
	if err != nil {
		return fmt.Errorf("failed to query pending notes: %v", err)
	}
	defer rows.Close()

	var pendingNotes []ScheduledNote
	for rows.Next() {
		var note ScheduledNote
		if err := rows.Scan(
			&note.ID,
			&note.CreatedAt,
			&note.UpdatedAt,
			&note.ProfileID,
			&note.Content,
			&note.ScheduledFor,
			&note.PublishedAt,
			&note.Status,
			&note.RelayURLs,
			&note.EventID,
			&note.ErrorMessage,
			&note.Signature,
			&note.SignedEvent,
		); err != nil {
			log.Printf("Error scanning note: %v", err)
			continue
		}
		pendingNotes = append(pendingNotes, note)
	}

	if err = rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %v", err)
	}

	log.Printf("Found %d pending notes to process", len(pendingNotes))
	if len(pendingNotes) == 0 {
		return nil
	}

	// Process notes in parallel with a worker pool
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxWorkers)
	
	for _, note := range pendingNotes {
		wg.Add(1)
		go func(n ScheduledNote) {
			defer wg.Done()
			sem <- struct{}{} // Acquire semaphore
			defer func() { <-sem }() // Release semaphore
			
			if err := processNote(ctx, pool, n); err != nil {
				log.Printf("Error processing note %s: %v", n.ID, err)
			}
		}(note)
	}
	
	// Wait for all goroutines to finish
	wg.Wait()
	
	return nil
}

func processNote(ctx context.Context, pool *pgxpool.Pool, note ScheduledNote) error {
	log.Printf("Processing note ID: %s, scheduled for: %v", note.ID, note.ScheduledFor.Format(time.RFC3339))
	
	// Unmarshal the signed event
	var event nostr.Event
	if note.SignedEvent != nil {
		if err := json.Unmarshal([]byte(*note.SignedEvent), &event); err != nil {
			errMsg := fmt.Sprintf("Failed to unmarshal signed event: %v", err)
			log.Println(errMsg)
			return updateNoteStatus(ctx, pool, note.ID, "failed", errMsg)
		}
	} else {
		errMsg := "Signed event is null"
		log.Println(errMsg)
		return updateNoteStatus(ctx, pool, note.ID, "failed", errMsg)
	}
	
	// Send the event to all specified relays
	successCount := 0
	var lastError error
	
	for _, relayURL := range note.RelayURLs {
		log.Printf("Sending note %s to relay: %s", note.ID, relayURL)
		
		// Connect to relay
		relay, err := nostr.RelayConnect(ctx, relayURL)
		if err != nil {
			log.Printf("Failed to connect to relay %s: %v", relayURL, err)
			lastError = err
			continue
		}
		
		// Create a timeout context for publishing
		publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		
		// Publish the event
		err = relay.Publish(publishCtx, event)
		cancel()
		relay.Close()
		
		if err != nil {
			log.Printf("Failed to publish to relay %s: %v", relayURL, err)
			lastError = err
			continue
		}
		
		// Successfully published to this relay
		log.Printf("Successfully published note %s to relay %s", note.ID, relayURL)
		successCount++
	}
	
	now := time.Now().UTC()
	
	// Update the note status based on the results
	if successCount > 0 {
		log.Printf("Note %s published successfully to %d/%d relays", note.ID, successCount, len(note.RelayURLs))
		
		// Update as published
		query := `
			UPDATE scheduled_notes 
			SET status = 'published', 
				published_at = $1, 
				updated_at = $1,
				event_id = $2,
				error_message = CASE 
					WHEN $3 = '' THEN NULL 
					ELSE $3 
				END
			WHERE id = $4
		`
		
		errMsg := ""
		if lastError != nil && successCount < len(note.RelayURLs) {
			errMsg = fmt.Sprintf("Partially published (%d/%d relays). Last error: %v", 
				successCount, len(note.RelayURLs), lastError)
		}
		
		_, err := pool.Exec(ctx, query, now, event.ID, errMsg, note.ID)
		if err != nil {
			log.Printf("Error updating note %s as published: %v", note.ID, err)
			return err
		}
		
		return nil
	}
	
	// If we get here, all relays failed
	errMsg := fmt.Sprintf("Failed to publish to any relay. Last error: %v", lastError)
	log.Println(errMsg)
	return updateNoteStatus(ctx, pool, note.ID, "failed", errMsg)
}

func updateNoteStatus(ctx context.Context, pool *pgxpool.Pool, noteID, status, errorMessage string) error {
	now := time.Now().UTC()
	
	query := `
		UPDATE scheduled_notes 
		SET status = $1, 
			updated_at = $2, 
			error_message = $3,
			published_at = CASE 
				WHEN $1 = 'published' THEN $2 
				ELSE published_at 
			END
		WHERE id = $4
	`
	
	_, err := pool.Exec(ctx, query, status, now, errorMessage, noteID)
	if err != nil {
		log.Printf("Error updating note %s status to %s: %v", noteID, status, err)
		return err
	}
	
	log.Printf("Updated note %s status to %s", noteID, status)
	return nil
}

func getDBConnection(ctx context.Context) (*pgxpool.Pool, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable not set")
	}
	
	// Create connection pool config
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %v", err)
	}
	
	// Configure the connection pool
	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute
	
	// Disable prepared statements to avoid conflicts
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	
	// Create the connection pool
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %v", err)
	}
	
	// Verify the connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}
	
	return pool, nil
}
