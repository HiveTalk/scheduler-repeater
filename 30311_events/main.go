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

	log.Printf("Checking for events between %v and %v", timeMin, timeMax)

	// Create error group for parallel batch processing
	g, ctx := errgroup.WithContext(context.Background())

	// Fetch and process starting events in batches
	g.Go(func() error {
		var startingEvents []Event
		rows, err := pool.Query(ctx,
			`SELECT id, name, start_time, end_time, room_name, identifier, description, image_url
			FROM events 
			WHERE start_time >= $1 
			AND start_time <= $2 
			AND status != 'live:sent'`,
			timeMin, timeMax)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var event Event
			if err := rows.Scan(&event.ID, &event.Name, &event.StartTime, &event.EndTime,
				&event.RoomName, &event.Identifier, &event.Description, &event.Image); err != nil {
				log.Printf("Error scanning starting event: %v", err)
				continue
			}
			startingEvents = append(startingEvents, event)
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
		rows, err := pool.Query(ctx,
			`SELECT id, name, start_time, end_time, room_name, identifier, description, image_url
			FROM events 
			WHERE end_time >= $1 
			AND end_time <= $2 
			AND status != 'ended:sent'`,
			timeMin, timeMax)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var event Event
			if err := rows.Scan(&event.ID, &event.Name, &event.StartTime, &event.EndTime,
				&event.RoomName, &event.Identifier, &event.Description, &event.Image); err != nil {
				log.Printf("Error scanning ending event: %v", err)
				continue
			}
			endingEvents = append(endingEvents, event)
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
