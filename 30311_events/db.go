package events

import (
	"database/sql"
	"fmt"
	"time"
)

type Database struct {
	db *sql.DB
}

type EventStatus string

const (
	EventStatusPending   EventStatus = "pending"
	EventStatusProcessed EventStatus = "processed"
	EventStatusFailed    EventStatus = "failed"
)

func NewDatabase(connStr string) (*Database, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}
	return &Database{db: db}, nil
}

func (d *Database) GetUnprocessedEvents() ([]EventData, error) {
	query := `
		SELECT 
			profile_id, naddr_id, name, description, image_url, 
			start_time, end_time, is_paid_event, room_name, 
			identifier, nostr_pubkey
		FROM events
		WHERE 
			nostr_status = $1 
			AND identifier IS NOT NULL 
			AND naddr_id IS NOT NULL
		ORDER BY updated_at DESC
		LIMIT 100
	`

	rows, err := d.db.Query(query, EventStatusPending)
	if err != nil {
		return nil, fmt.Errorf("failed to query events pending update: %v", err)
	}
	defer rows.Close()

	var events []EventData
	for rows.Next() {
		var e EventData
		err := rows.Scan(
			&e.ProfileID, &e.NaddrID, &e.Name, &e.Description, &e.ImageURL,
			&e.StartTime, &e.EndTime, &e.IsPaidEvent, &e.RoomName,
			&e.Identifier, &e.NostrPubkey,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event row: %v", err)
		}
		events = append(events, e)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating event rows: %v", err)
	}

	return events, nil
}

func (d *Database) MarkEventAsProcessed(identifier string) error {
	query := `
		UPDATE events
		SET 
			nostr_status = $1,
			nostr_processed_at = $2,
			updated_at = $2
		WHERE identifier = $3
	`

	result, err := d.db.Exec(query, EventStatusProcessed, time.Now(), identifier)
	if err != nil {
		return fmt.Errorf("failed to mark event as processed: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting rows affected: %v", err)
	}

	if rows == 0 {
		return fmt.Errorf("no event found with identifier: %s", identifier)
	}

	return nil
}

func (d *Database) MarkEventAsFailed(identifier string, errorMsg string) error {
	query := `
		UPDATE events
		SET 
			nostr_status = $1,
			nostr_error = $2,
			updated_at = $3
		WHERE identifier = $4
	`

	result, err := d.db.Exec(query, EventStatusFailed, errorMsg, time.Now(), identifier)
	if err != nil {
		return fmt.Errorf("failed to mark event as failed: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting rows affected: %v", err)
	}

	if rows == 0 {
		return fmt.Errorf("no event found with identifier: %s", identifier)
	}

	return nil
}
