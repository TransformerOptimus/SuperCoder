package services

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
)

// SSEEvent represents a parsed Server-Sent Event.
type SSEEvent struct {
	Event string
	Data  []byte
}

// ParseSSEStream reads SSE events from reader, calling handler for each complete event.
// Returns on EOF, context cancellation, or handler error.
func ParseSSEStream(ctx context.Context, reader io.Reader, handler func(SSEEvent) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var eventType string
	var dataLines [][]byte

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()

		if line == "" {
			// Blank line = event boundary
			if len(dataLines) > 0 {
				data := bytes.Join(dataLines, []byte("\n"))
				evt := SSEEvent{
					Event: eventType,
					Data:  data,
				}
				if err := handler(evt); err != nil {
					return err
				}
			}
			eventType = ""
			dataLines = nil
			continue
		}

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			raw := strings.TrimPrefix(line, "data:")
			raw = strings.TrimPrefix(raw, " ")
			dataLines = append(dataLines, []byte(raw))
		}
		// Ignore comments (lines starting with ':') and other fields
	}

	// Process any remaining event data
	if len(dataLines) > 0 {
		data := bytes.Join(dataLines, []byte("\n"))
		evt := SSEEvent{
			Event: eventType,
			Data:  data,
		}
		if err := handler(evt); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("sse scanner error (last_event=%q): %w", eventType, err)
	}
	return nil
}
