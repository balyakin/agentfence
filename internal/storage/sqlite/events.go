package sqlite

import (
	"context"
	"fmt"

	"github.com/agentfence/agentfence/internal/domain"
)

func (s *Store) InsertEvent(ctx context.Context, event domain.RunEvent) error {
	if event.Ts.IsZero() {
		event.Ts = s.clock.Now()
	}
	if event.DataJSON == "" {
		event.DataJSON = "{}"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO run_events(run_id, ts, level, event_type, message, data_json)
VALUES (?, ?, ?, ?, ?, ?)`,
		event.RunID, formatTime(event.Ts), event.Level, event.EventType, event.Message, event.DataJSON)
	if err != nil {
		return fmt.Errorf("insert run event: %w", err)
	}
	return nil
}
