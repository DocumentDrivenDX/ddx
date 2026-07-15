package agent

import agentlib "github.com/easel/fizeau"

const (
	BillingModeUnknown      = "unknown"
	BillingModePaid         = "paid"
	BillingModeSubscription = "subscription"
	BillingModeLocal        = "local"
)

// BillingPresentationMode maps Fizeau's public billing evidence to the
// presentation buckets used by DDx. The raw value is persisted separately;
// empty or unfamiliar values deliberately remain unknown.
func BillingPresentationMode(billing string) string {
	switch agentlib.BillingModel(billing) {
	case agentlib.BillingModelPerToken:
		return BillingModePaid
	case agentlib.BillingModelSubscription:
		return BillingModeSubscription
	case agentlib.BillingModelFixed:
		return BillingModeLocal
	default:
		return BillingModeUnknown
	}
}

func ValidateBillingMode(mode string) bool {
	switch mode {
	case BillingModeUnknown, BillingModePaid, BillingModeSubscription, BillingModeLocal:
		return true
	default:
		return false
	}
}
