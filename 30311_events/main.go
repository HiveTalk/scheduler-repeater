package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

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

	// Diagnostic query to check ALL events in the database
	// rows, err := pool.Query(context.Background(), `
	// 	SELECT
	// 		id,
	// 		name,
	// 		start_time,
	// 		end_time,
	// 		status,
	// 		room_name,
	// 		identifier,
	// 		description,
	// 		image_url
	// 	FROM events
	// 	ORDER BY start_time DESC
	// 	LIMIT 5`)
	// if err != nil {
	// 	log.Printf("Diagnostic query failed: %v", err)
	// } else {
	// 	defer rows.Close()
	// }

	// Also check if we can count total events
	// var count int
	// err = pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM events").Scan(&count)
	// if err != nil {
	// 	log.Printf("Error counting events: %v", err)
	// } else {
	// 	log.Printf("\nTotal events in database: %d", count)
	// }

	// Show events specifically in our time window
	timeWindowRows, err := pool.Query(context.Background(), `
		SELECT COUNT(*)
		FROM events
		WHERE start_time >= $1
		AND start_time <= $2`, timeMin, timeMax)
	if err != nil {
		log.Printf("Time window count query failed: %v", err)
	} else {
		defer timeWindowRows.Close()
		if timeWindowRows.Next() {
			var windowCount int
			if err := timeWindowRows.Scan(&windowCount); err != nil {
				log.Printf("Error scanning time window count: %v", err)
			} else {
				log.Printf("\nEvents in current time window: %d", windowCount)
			}
		}
	}

	// Create error group for parallel batch processing
	g, ctx := errgroup.WithContext(context.Background())

	// Fetch and process starting events in batches
	g.Go(func() error {
		var startingEvents []Event
		// First, let's check what events exist in our time window without any status filter
		debugQuery := `
			SELECT id, name, start_time, end_time, status
			FROM events 
			WHERE start_time >= $1 
			AND start_time <= $2`

		debugRows, err := pool.Query(ctx, debugQuery, timeMin, timeMax)
		if err != nil {
			log.Printf("Debug query failed: %v", err)
		} else {
			defer debugRows.Close()
			log.Printf("\n=== Events in time window (before status filter) ===")
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
			log.Printf("Added event to startingEvents: ID=%s, Name=%s, StartTime=%v, Status=%v",
				event.ID, event.Name, event.StartTime.Format(time.RFC3339), stringPtrValue(event.Status))
		}

		if err = rows.Err(); err != nil {
			log.Printf("Error iterating starting events: %v", err)
		}

		log.Printf("Found %d starting events", len(startingEvents))
		for _, e := range startingEvents {
			log.Printf("Starting event: ID=%s, Name=%s, Room=%v, StartTime=%v, Status=%v",
				e.ID, e.Name, stringPtrValue(e.RoomName), e.StartTime.Format(time.RFC3339), stringPtrValue(e.Status))
		}

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
		// First, let's check what events exist in our time window without any status filter
		debugQuery := `
			SELECT id, name, start_time, end_time, status
			FROM events 
			WHERE end_time >= $1 
			AND end_time <= $2`

		debugRows, err := pool.Query(ctx, debugQuery, timeMin, timeMax)
		if err != nil {
			log.Printf("Debug query failed: %v", err)
		} else {
			defer debugRows.Close()
			log.Printf("\n=== Events in time window (before status filter) ===")
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
			log.Printf("Added event to endingEvents: ID=%s, Name=%s, EndTime=%v, Status=%v",
				event.ID, event.Name, event.EndTime.Format(time.RFC3339), stringPtrValue(event.Status))
		}

		if err = rows.Err(); err != nil {
			log.Printf("Error iterating ending events: %v", err)
		}

		log.Printf("Found %d ending events", len(endingEvents))
		for _, e := range endingEvents {
			log.Printf("Ending event: ID=%s, Name=%s, Room=%v, EndTime=%v, Status=%v",
				e.ID, e.Name, stringPtrValue(e.RoomName), e.EndTime.Format(time.RFC3339), stringPtrValue(e.Status))
		}

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
