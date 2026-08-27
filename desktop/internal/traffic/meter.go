package traffic

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Protocol string

const (
	ProtocolHTTP   Protocol = "http"
	ProtocolSOCKS5 Protocol = "socks5"
)

type ProtocolSnapshot struct {
	Connections   int64 `json:"connections"`
	UploadBytes   int64 `json:"uploadBytes"`
	DownloadBytes int64 `json:"downloadBytes"`
}

type Snapshot struct {
	StartedAtMillis        int64            `json:"startedAtMillis"`
	ActiveConnections      int64            `json:"activeConnections"`
	TotalConnections       int64            `json:"totalConnections"`
	UploadBytes            int64            `json:"uploadBytes"`
	DownloadBytes          int64            `json:"downloadBytes"`
	UploadBytesPerSecond   int64            `json:"uploadBytesPerSecond"`
	DownloadBytesPerSecond int64            `json:"downloadBytesPerSecond"`
	HTTP                   ProtocolSnapshot `json:"http"`
	SOCKS5                 ProtocolSnapshot `json:"socks5"`
}

type counters struct {
	connections atomic.Int64
	upload      atomic.Int64
	download    atomic.Int64
}

type Meter struct {
	startedAt time.Time
	active    atomic.Int64
	total     counters
	http      counters
	socks5    counters

	rateMu       sync.Mutex
	lastSampleAt time.Time
	lastUpload   int64
	lastDownload int64
}

func NewMeter() *Meter {
	now := time.Now()
	return &Meter{startedAt: now, lastSampleAt: now}
}

func (m *Meter) Wrap(connection net.Conn, protocol Protocol) net.Conn {
	m.active.Add(1)
	m.total.connections.Add(1)
	m.protocol(protocol).connections.Add(1)
	return &meteredConnection{
		Conn:     connection,
		meter:    m,
		protocol: protocol,
	}
}

func (m *Meter) Snapshot() Snapshot {
	now := time.Now()
	upload := m.total.upload.Load()
	download := m.total.download.Load()

	m.rateMu.Lock()
	elapsed := now.Sub(m.lastSampleAt).Seconds()
	startedAtMillis := m.startedAt.UnixMilli()
	var uploadRate, downloadRate int64
	if elapsed > 0 {
		uploadRate = int64(float64(upload-m.lastUpload) / elapsed)
		downloadRate = int64(float64(download-m.lastDownload) / elapsed)
	}
	m.lastSampleAt = now
	m.lastUpload = upload
	m.lastDownload = download
	m.rateMu.Unlock()

	return Snapshot{
		StartedAtMillis:        startedAtMillis,
		ActiveConnections:      m.active.Load(),
		TotalConnections:       m.total.connections.Load(),
		UploadBytes:            upload,
		DownloadBytes:          download,
		UploadBytesPerSecond:   uploadRate,
		DownloadBytesPerSecond: downloadRate,
		HTTP:                   snapshotCounters(&m.http),
		SOCKS5:                 snapshotCounters(&m.socks5),
	}
}

func (m *Meter) Reset() {
	m.total.connections.Store(0)
	m.total.upload.Store(0)
	m.total.download.Store(0)
	resetCounters(&m.http)
	resetCounters(&m.socks5)
	now := time.Now()
	m.rateMu.Lock()
	m.startedAt = now
	m.lastSampleAt = now
	m.lastUpload = 0
	m.lastDownload = 0
	m.rateMu.Unlock()
}

func (m *Meter) protocol(protocol Protocol) *counters {
	if protocol == ProtocolSOCKS5 {
		return &m.socks5
	}
	return &m.http
}

func (m *Meter) recordUpload(protocol Protocol, count int) {
	if count <= 0 {
		return
	}
	m.total.upload.Add(int64(count))
	m.protocol(protocol).upload.Add(int64(count))
}

func (m *Meter) recordDownload(protocol Protocol, count int) {
	if count <= 0 {
		return
	}
	m.total.download.Add(int64(count))
	m.protocol(protocol).download.Add(int64(count))
}

func snapshotCounters(value *counters) ProtocolSnapshot {
	return ProtocolSnapshot{
		Connections:   value.connections.Load(),
		UploadBytes:   value.upload.Load(),
		DownloadBytes: value.download.Load(),
	}
}

func resetCounters(value *counters) {
	value.connections.Store(0)
	value.upload.Store(0)
	value.download.Store(0)
}

type meteredConnection struct {
	net.Conn
	meter    *Meter
	protocol Protocol
	close    sync.Once
}

func (c *meteredConnection) Read(buffer []byte) (int, error) {
	count, err := c.Conn.Read(buffer)
	c.meter.recordUpload(c.protocol, count)
	return count, err
}

func (c *meteredConnection) Write(buffer []byte) (int, error) {
	count, err := c.Conn.Write(buffer)
	c.meter.recordDownload(c.protocol, count)
	return count, err
}

func (c *meteredConnection) Close() error {
	c.close.Do(func() { c.meter.active.Add(-1) })
	return c.Conn.Close()
}
