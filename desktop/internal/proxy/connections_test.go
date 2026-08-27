package proxy

import (
	"net"
	"testing"
	"time"
)

func TestConnectionTrackerClosesEveryActiveConnection(t *testing.T) {
	tracker := NewConnectionTracker()
	client, server := net.Pipe()
	tracked := tracker.Wrap(server)

	if closed := tracker.CloseAll(); closed != 1 {
		t.Fatalf("closed = %d, want 1", closed)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("client connection remained open")
	}
	if err := tracked.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if closed := tracker.CloseAll(); closed != 0 {
		t.Fatalf("closed after cleanup = %d, want 0", closed)
	}
	_ = client.Close()
}
