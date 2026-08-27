package adapter

import "strings"

const minimumUSBScore = 55

var strongUSBHints = []string{
	"remote ndis",
	"rndis",
	"usb ncm",
	"ethernet gadget",
	"usb ethernet",
	"android usb",
}

var weakUSBHints = []string{
	"usb",
	"ncm",
	"android",
	"xiaomi",
	"samsung",
	"oneplus",
	"pixel",
	"huawei",
	"oppo",
	"vivo",
	"gadget",
}

var virtualAdapterHints = []string{
	"hyper-v",
	"virtualbox",
	"vmware",
	"wi-fi direct",
	"bluetooth",
	"wan miniport",
	"loopback",
	"tunnel",
	"vpn",
	"wsl",
	"vethernet",
	"tap-",
	"tun",
}

func ScoreUSBAdapter(value Adapter) (score int, candidate bool) {
	text := strings.ToLower(value.Name + " " + value.Description)
	for _, hint := range strongUSBHints {
		if strings.Contains(text, hint) {
			score += 90
			break
		}
	}
	for _, hint := range weakUSBHints {
		if strings.Contains(text, hint) {
			score += 28
			break
		}
	}
	for _, hint := range virtualAdapterHints {
		if strings.Contains(text, hint) {
			score -= 120
			break
		}
	}
	if value.IsUp() {
		score += 20
	}
	if len(value.IPv4) > 0 {
		score += 8
	}
	if len(value.Gateways) > 0 {
		score += 12
	}
	if value.MACAddress != "" {
		score += 3
	}
	return score, score >= minimumUSBScore
}
