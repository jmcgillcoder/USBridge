package exclusivenet

const (
	policyFamilyIPv4 = 4
	policyFamilyIPv6 = 6

	policyActionPermit = iota + 1
	policyActionBlock

	conditionInterface = iota + 1
	conditionAppID
	conditionProtocol
	conditionLocalPort
	conditionRemotePort
	conditionICMPType
	conditionICMPCode

	ipProtocolUDP    = 17
	ipProtocolICMPv6 = 58
)

type policyCondition struct {
	kind  int
	value uint16
}

type policyRule struct {
	name       string
	family     int
	weight     uint8
	action     int
	hardAction bool
	conditions []policyCondition
}

func buildPolicyRules() []policyRule {
	interfaceCondition := policyCondition{kind: conditionInterface}
	appCondition := policyCondition{kind: conditionAppID}
	protocolUDP := policyCondition{kind: conditionProtocol, value: ipProtocolUDP}
	protocolICMPv6 := policyCondition{kind: conditionProtocol, value: ipProtocolICMPv6}
	icmpCodeZero := policyCondition{kind: conditionICMPCode, value: 0}

	rules := []policyRule{
		{
			name:       "Permit USBridge on phone USB (IPv4)",
			family:     policyFamilyIPv4,
			weight:     15,
			action:     policyActionPermit,
			conditions: []policyCondition{interfaceCondition, appCondition},
		},
		{
			name:       "Permit USBridge on phone USB (IPv6)",
			family:     policyFamilyIPv6,
			weight:     15,
			action:     policyActionPermit,
			conditions: []policyCondition{interfaceCondition, appCondition},
		},
		{
			name:   "Permit DHCP on phone USB (IPv4)",
			family: policyFamilyIPv4,
			weight: 12,
			action: policyActionPermit,
			conditions: []policyCondition{
				interfaceCondition,
				protocolUDP,
				{kind: conditionLocalPort, value: 68},
				{kind: conditionRemotePort, value: 67},
			},
		},
		{
			name:   "Permit DHCP on phone USB (IPv6)",
			family: policyFamilyIPv6,
			weight: 12,
			action: policyActionPermit,
			conditions: []policyCondition{
				interfaceCondition,
				protocolUDP,
				{kind: conditionLocalPort, value: 546},
				{kind: conditionRemotePort, value: 547},
			},
		},
	}

	// Router/neighbor discovery and multicast listener reports keep IPv6 usable.
	for _, messageType := range []uint16{131, 132, 133, 135, 136, 143} {
		rules = append(rules, policyRule{
			name:   "Permit IPv6 link maintenance on phone USB",
			family: policyFamilyIPv6,
			weight: 12,
			action: policyActionPermit,
			conditions: []policyCondition{
				interfaceCondition,
				protocolICMPv6,
				{kind: conditionICMPType, value: messageType},
				icmpCodeZero,
			},
		})
	}

	rules = append(rules,
		policyRule{
			name:       "Block other applications on phone USB (IPv4)",
			family:     policyFamilyIPv4,
			weight:     0,
			action:     policyActionBlock,
			hardAction: true,
			conditions: []policyCondition{interfaceCondition},
		},
		policyRule{
			name:       "Block other applications on phone USB (IPv6)",
			family:     policyFamilyIPv6,
			weight:     0,
			action:     policyActionBlock,
			hardAction: true,
			conditions: []policyCondition{interfaceCondition},
		},
	)
	return rules
}
