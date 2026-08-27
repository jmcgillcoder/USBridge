package traffic

import (
	"io"
	"net"
	"testing"
)

func TestMeterTracksBothDirections(t *testing.T) {
	meter := NewMeter()
	client, server := net.Pipe()
	tracked := meter.Wrap(server, ProtocolHTTP)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 4)
		_, _ = io.ReadFull(tracked, buffer)
		_, _ = tracked.Write([]byte("reply"))
		_ = tracked.Close()
	}()

	_, _ = client.Write([]byte("ping"))
	response := make([]byte, 5)
	_, _ = io.ReadFull(client, response)
	_ = client.Close()
	<-done

	snapshot := meter.Snapshot()
	if snapshot.UploadBytes != 4 || snapshot.DownloadBytes != 5 {
		t.Fatalf("unexpected totals: %+v", snapshot)
	}
	if snapshot.HTTP.Connections != 1 || snapshot.ActiveConnections != 0 {
		t.Fatalf("unexpected connection counts: %+v", snapshot)
	}
}
