package androidclient

import "time"

type StatusResponse struct {
	Version    string          `json:"version"`
	Root       RootStatus      `json:"root"`
	USB        USBStatus       `json:"usb"`
	Mobile     MobileStatus    `json:"mobile"`
	IPMode     string          `json:"ipMode"`
	PublicIP   PublicIPStatus  `json:"publicIp"`
	Traffic    TrafficResponse `json:"traffic"`
	ObservedAt time.Time       `json:"observedAt"`
}

type RootStatus struct {
	Granted        bool   `json:"granted"`
	Implementation string `json:"implementation,omitempty"`
}

type USBStatus struct {
	Connected        bool     `json:"connected"`
	TetheringEnabled bool     `json:"tetheringEnabled"`
	Upstream         string   `json:"upstream,omitempty"`
	CellularUpstream bool     `json:"cellularUpstream"`
	Interfaces       []string `json:"interfaces"`
}

type MobileStatus struct {
	Connected     bool     `json:"connected"`
	IPv4Available *bool    `json:"ipv4Available,omitempty"`
	IPv6Available *bool    `json:"ipv6Available,omitempty"`
	Interfaces    []string `json:"interfaces"`
}

type PublicIPStatus struct {
	IPv4 string `json:"ipv4,omitempty"`
	IPv6 string `json:"ipv6,omitempty"`
}

type TrafficResponse struct {
	InterfaceName          string `json:"interfaceName,omitempty"`
	UploadBytesPerSecond   int64  `json:"uploadBytesPerSecond"`
	DownloadBytesPerSecond int64  `json:"downloadBytesPerSecond"`
	SessionUploadBytes     int64  `json:"sessionUploadBytes"`
	SessionDownloadBytes   int64  `json:"sessionDownloadBytes"`
	TodayUploadBytes       int64  `json:"todayUploadBytes"`
	TodayDownloadBytes     int64  `json:"todayDownloadBytes"`
	MonthUploadBytes       int64  `json:"monthUploadBytes"`
	MonthDownloadBytes     int64  `json:"monthDownloadBytes"`
}

type OperationResponse struct {
	OK                  bool            `json:"ok"`
	Message             string          `json:"message"`
	Before              *PublicIPStatus `json:"before,omitempty"`
	After               *PublicIPStatus `json:"after,omitempty"`
	CommandSucceeded    *bool           `json:"commandSucceeded,omitempty"`
	NetworkDisconnected *bool           `json:"networkDisconnected,omitempty"`
	NetworkRecovered    *bool           `json:"networkRecovered,omitempty"`
	IPChanged           *bool           `json:"ipChanged,omitempty"`
}
