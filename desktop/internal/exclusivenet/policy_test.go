package exclusivenet

import "testing"

func TestBuildPolicyRulesProtectsBothAddressFamilies(t *testing.T) {
	rules := buildPolicyRules()
	if len(rules) != 12 {
		t.Fatalf("expected 12 policy rules, got %d", len(rules))
	}

	permitSelf := map[int]bool{}
	blockRest := map[int]bool{}
	for _, rule := range rules {
		if !hasCondition(rule, conditionInterface) {
			t.Fatalf("rule %q is not scoped to the selected interface", rule.name)
		}
		if rule.action == policyActionPermit && hasCondition(rule, conditionAppID) {
			permitSelf[rule.family] = true
			if rule.hardAction {
				t.Fatalf("USBridge permit %q must remain subject to other firewall policy", rule.name)
			}
		}
		if rule.action == policyActionBlock {
			blockRest[rule.family] = true
			if !rule.hardAction {
				t.Fatalf("catch-all block %q must be a hard action", rule.name)
			}
		}
	}

	for _, family := range []int{policyFamilyIPv4, policyFamilyIPv6} {
		if !permitSelf[family] || !blockRest[family] {
			t.Fatalf("address family %d is missing permit-self or block-rest", family)
		}
	}
}

func TestPolicyInfrastructurePermitsPrecedeBlocks(t *testing.T) {
	rules := buildPolicyRules()
	for _, rule := range rules {
		if rule.action == policyActionBlock {
			continue
		}
		if rule.weight == 0 {
			t.Fatalf("permit rule %q does not outrank the catch-all block", rule.name)
		}
	}
}

func hasCondition(rule policyRule, kind int) bool {
	for _, condition := range rule.conditions {
		if condition.kind == kind {
			return true
		}
	}
	return false
}
