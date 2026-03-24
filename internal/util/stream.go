package util

import "time"

// StreamTracker manages throttled progressive message streaming.
// It tracks what portion of the text has already been sent so that
// each platform callback only transmits the new delta.
type StreamTracker struct {
	lastSend    time.Time
	lastSentLen int
	streamed    bool
	interval    time.Duration
}

// NewStreamTracker creates a StreamTracker with the given throttle interval.
func NewStreamTracker(interval time.Duration) *StreamTracker {
	return &StreamTracker{interval: interval}
}

// ShouldSend checks whether enough time has passed and there is new text
// to send. Returns the new portion and true, or ("", false) if skipped.
func (t *StreamTracker) ShouldSend(text string) (string, bool) {
	if time.Since(t.lastSend) < t.interval {
		return "", false
	}
	newPortion := text[t.lastSentLen:]
	if newPortion == "" {
		return "", false
	}
	return newPortion, true
}

// MarkSent records that text up to fullLen has been delivered.
func (t *StreamTracker) MarkSent(fullLen int) {
	t.lastSentLen = fullLen
	t.lastSend = time.Now()
	t.streamed = true
}

// FinalText returns the portion of the full response that was never
// streamed. Returns ("", false) if everything was already sent.
func (t *StreamTracker) FinalText(response string) (string, bool) {
	if t.lastSentLen >= len(response) {
		return "", false
	}
	if t.lastSentLen > 0 {
		return response[t.lastSentLen:], true
	}
	return response, true
}

// Streamed reports whether any chunks were sent during streaming.
func (t *StreamTracker) Streamed() bool { return t.streamed }

// IsFirstChunk reports whether no chunks have been sent yet.
func (t *StreamTracker) IsFirstChunk() bool { return t.lastSentLen == 0 }
