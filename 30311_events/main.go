package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

const (
	twoMinutesMs = 2 * 60 * 1000 // 2 minutes in milliseconds
	batchSize    = 25            // Reduced from 50 to 25 for smaller batches
	maxWorkers   = 2             // Reduced from 5 to 2 for 1-2 vCPU environments
)

type EventBatch struct {
	Events []Event
	Status string
}

func processBatch(batch EventBatch) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(batch.Events))
	semaphore := make(chan struct{}, maxWorkers)

	for _, event := range batch.Events {
		wg.Add(1)
		go func(e Event) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			if err := sendNewEvent(e, batch.Status); err != nil {
				log.Printf("Failed to process event %s: %v", e.ID, err)
				errChan <- err
			} else {
				log.Printf("Successfully processed event: %s (ID: %s)", e.Name, e.ID)
			}
		}(event)
	}

	// Wait for all goroutines to finish
	wg.Wait()
	close(errChan)

	// Collect any errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("batch processing encountered %d errors", len(errs))
	}
	return nil
}

func fetchUpcomingEvents() error {
	pool, err := getSupabaseConnection()
	if err != nil {
		return err
	}
	defer pool.Close()

	currentTime := time.Now()
	timeMin := currentTime.Add(-time.Duration(twoMinutesMs) * time.Millisecond)
	timeMax := currentTime.Add(time.Duration(twoMinutesMs) * time.Millisecond)

	log.Printf("Checking for events between %v and %v", timeMin.Format(time.RFC3339), timeMax.Format(time.RFC3339))

	// Create error group for parallel batch processing
	g, ctx := errgroup.WithContext(context.Background())

	// Fetch and process starting events in batches
	g.Go(func() error {
		var startingEvents []Event

		debugEventsInTimeWindow(ctx, pool, timeMin, timeMax, "starting")

		// Now run the actual query
		query := `
			SELECT id, name, start_time, end_time, room_name, identifier, description, image_url, status
			FROM events 
			WHERE start_time >= $1 
			AND start_time <= $2 
			AND (status IS NULL OR status NOT IN ('live:sent', 'ended:sent'))`

		log.Printf("\nRunning starting events query with time window: %v to %v",
			timeMin.Format(time.RFC3339),
			timeMax.Format(time.RFC3339))

		rows, err := pool.Query(ctx, query, timeMin, timeMax)
		if err != nil {
			return fmt.Errorf("starting events query failed: %v", err)
		}
		defer rows.Close()

		for rows.Next() {
			var event Event
			if err := rows.Scan(
				&event.ID,
				&event.Name,
				&event.StartTime,
				&event.EndTime,
				&event.RoomName,
				&event.Identifier,
				&event.Description,
				&event.Image,
				&event.Status,
			); err != nil {
				log.Printf("Error scanning starting event: %v", err)
				continue
			}
			startingEvents = append(startingEvents, event)
			logEventProcessing(event, "starting")
		}

		if err = rows.Err(); err != nil {
			log.Printf("Error iterating starting events: %v", err)
		}

		log.Printf("Found %d starting events", len(startingEvents))

		// Process starting events in batches
		for i := 0; i < len(startingEvents); i += batchSize {
			end := i + batchSize
			if end > len(startingEvents) {
				end = len(startingEvents)
			}
			batch := EventBatch{
				Events: startingEvents[i:end],
				Status: "live",
			}
			if err := processBatch(batch); err != nil {
				return fmt.Errorf("error processing starting events batch: %v", err)
			}
		}
		return nil
	})

	// Fetch and process ending events in batches
	g.Go(func() error {
		var endingEvents []Event

		debugEventsInTimeWindow(ctx, pool, timeMin, timeMax, "ending")

		// Now run the actual query
		query := `
			SELECT id, name, start_time, end_time, room_name, identifier, description, image_url, status
			FROM events 
			WHERE end_time >= $1 
			AND end_time <= $2 
			AND (status IS NULL OR status NOT IN ('live:sent', 'ended:sent'))`

		log.Printf("\nRunning ending events query with time window: %v to %v",
			timeMin.Format(time.RFC3339),
			timeMax.Format(time.RFC3339))

		rows, err := pool.Query(ctx, query, timeMin, timeMax)
		if err != nil {
			return fmt.Errorf("ending events query failed: %v", err)
		}
		defer rows.Close()

		for rows.Next() {
			var event Event
			if err := rows.Scan(
				&event.ID,
				&event.Name,
				&event.StartTime,
				&event.EndTime,
				&event.RoomName,
				&event.Identifier,
				&event.Description,
				&event.Image,
				&event.Status,
			); err != nil {
				log.Printf("Error scanning ending event: %v", err)
				continue
			}
			endingEvents = append(endingEvents, event)
			logEventProcessing(event, "ending")
		}

		if err = rows.Err(); err != nil {
			log.Printf("Error iterating ending events: %v", err)
		}

		log.Printf("Found %d ending events", len(endingEvents))

		// Process ending events in batches
		for i := 0; i < len(endingEvents); i += batchSize {
			end := i + batchSize
			if end > len(endingEvents) {
				end = len(endingEvents)
			}
			batch := EventBatch{
				Events: endingEvents[i:end],
				Status: "ended",
			}
			if err := processBatch(batch); err != nil {
				return fmt.Errorf("error processing ending events batch: %v", err)
			}
		}
		return nil
	})

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return fmt.Errorf("error in parallel processing: %v", err)
	}

	return nil
}

func main() {
	if err := fetchUpcomingEvents(); err != nil {
		log.Fatalf("Error fetching upcoming events: %v", err)
	}
}

func stringPtrValue(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// Debug helper functions
func debugEventsInTimeWindow(ctx context.Context, pool *pgxpool.Pool, timeMin, timeMax time.Time, eventType string) {
	debugQuery := `
		SELECT id, name, start_time, end_time, status
		FROM events 
		WHERE %s >= $1 
		AND %s <= $2`

	// Format query based on event type
	timeField := "start_time"
	if eventType == "ending" {
		timeField = "end_time"
	}
	debugQuery = fmt.Sprintf(debugQuery, timeField, timeField)

	debugRows, err := pool.Query(ctx, debugQuery, timeMin, timeMax)
	if err != nil {
		log.Printf("Debug query failed: %v", err)
		return
	}
	defer debugRows.Close()

	log.Printf("\n=== %s Events in time window (before status filter) ===", strings.Title(eventType))
	for debugRows.Next() {
		var id, name string
		var startTime, endTime time.Time
		var status *string
		if err := debugRows.Scan(&id, &name, &startTime, &endTime, &status); err != nil {
			log.Printf("Error scanning debug row: %v", err)
			continue
		}
		log.Printf("Found event: ID=%s, Name=%s, StartTime=%v, Status=%v",
			id, name, startTime.Format(time.RFC3339), stringPtrValue(status))
	}
}

func logEventProcessing(event Event, eventType string) {
	if eventType == "starting" {
		log.Printf("Added %s event: ID=%s, Name=%s, StartTime=%v, Status=%v",
			eventType,
			event.ID,
			event.Name,
			event.StartTime.Format(time.RFC3339),
			stringPtrValue(event.Status))
	} else {
		log.Printf("Added %s event: ID=%s, Name=%s, EndTime=%v, Status=%v",
			eventType,
			event.ID,
			event.Name,
			event.EndTime.Format(time.RFC3339),
			stringPtrValue(event.Status))
	}
}
