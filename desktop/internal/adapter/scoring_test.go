package adapter

import "testing"

func TestScoreUSBAdapter(t *testing.T) {
	tests := []struct {
		name      string
		adapter   Adapter
		candidate bool
	}{
		{
			name: "Android RNDIS",
			adapter: Adapter{
				Name:        "以太网 3",
				Description: "Remote NDIS Compatible Device",
				Status:      "Up",
				IPv4:        []string{"192.168.42.10"},
				Gateways:    []string{"192.168.42.129"},
			},
			candidate: true,
		},
		{
			name: "Hyper-V",
			adapter: Adapter{
				Name:        "vEthernet (Default Switch)",
				Description: "Hyper-V Virtual Ethernet Adapter",
				Status:      "Up",
				IPv4:        []string{"172.20.0.1"},
			},
			candidate: false,
		},
		{
			name: "Physical ethernet",
			adapter: Adapter{
				Name:        "Ethernet",
				Description: "Realtek Gaming 2.5GbE Family Controller",
				Status:      "Up",
			},
			candidate: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, candidate := ScoreUSBAdapter(test.adapter)
			if candidate != test.candidate {
				t.Fatalf("candidate = %v, want %v", candidate, test.candidate)
			}
		})
	}
}
