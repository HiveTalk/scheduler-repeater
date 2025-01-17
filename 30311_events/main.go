package main

import (
	"context"
	"log"
	"time"
)

const (
	twoMinutesMs = 2 * 60 * 1000 // 2 minutes in milliseconds
)

func fetchUpcomingEvents() error {
	conn, err := getSupabaseConnection()
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	currentTime := time.Now()
	timeMin := currentTime.Add(-time.Duration(twoMinutesMs) * time.Millisecond)
	timeMax := currentTime.Add(time.Duration(twoMinutesMs) * time.Millisecond)

	log.Printf("Checking for events between %v and %v", timeMin, timeMax)

	// Fetch starting events
	var startingEvents []Event
	rows, err := conn.Query(context.Background(),
		`SELECT id, name, start_time, end_time, room_name, identifier, description, image_url
		FROM events 
		WHERE start_time >= $1 
		AND start_time <= $2 
		AND status != 'live:sent'`,
		timeMin, timeMax)
	if err != nil {
		return err
	}
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.Name, &event.StartTime, &event.EndTime,
			&event.RoomName, &event.Identifier, &event.Description, &event.Image); err != nil {
			log.Printf("Error scanning starting event: %v", err)
			continue
		}
		startingEvents = append(startingEvents, event)
	}
	rows.Close()

	log.Printf("Found %d starting events", len(startingEvents))

	// Fetch ending events
	var endingEvents []Event
	rows, err = conn.Query(context.Background(),
		`SELECT id, name, start_time, end_time, room_name, identifier, description, image_url
		FROM events 
		WHERE end_time >= $1 
		AND end_time <= $2 
		AND status != 'ended:sent'`,
		timeMin, timeMax)
	if err != nil {
		return err
	}
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.Name, &event.StartTime, &event.EndTime,
			&event.RoomName, &event.Identifier, &event.Description, &event.Image); err != nil {
			log.Printf("Error scanning ending event: %v", err)
			continue
		}
		endingEvents = append(endingEvents, event)
	}
	rows.Close()

	log.Printf("Found %d ending events", len(endingEvents))

	// Process starting events
	for _, event := range startingEvents {
		if err := sendNewEvent(event, "live"); err != nil {
			log.Printf("Failed to send starting event %s: %v", event.ID, err)
		} else {
			log.Printf("Successfully processed starting event: %s (ID: %s)", event.Name, event.ID)
		}
	}

	// Process ending events
	for _, event := range endingEvents {
		if err := sendNewEvent(event, "ended"); err != nil {
			log.Printf("Failed to send ending event %s: %v", event.ID, err)
		} else {
			log.Printf("Successfully processed ending event: %s (ID: %s)", event.Name, event.ID)
		}
	}

	return nil
}

func main() {
	if err := fetchUpcomingEvents(); err != nil {
		log.Fatalf("Error fetching upcoming events: %v", err)
	}
}
