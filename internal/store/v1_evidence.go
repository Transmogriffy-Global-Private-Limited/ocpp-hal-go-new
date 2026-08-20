package store

import "time"

// Charger clocks are evidence, never the authority for expiry or enforcement.
// This accommodates modest device drift while rejecting timestamps that could
// otherwise backdate authorization or move a durable session arbitrarily.
const v1MaxProtocolClockSkew = 15 * time.Minute

func plausibleV1ProtocolTime(protocolAt, receivedAt time.Time) bool {
	if protocolAt.IsZero() || receivedAt.IsZero() {
		return false
	}
	delta := protocolAt.Sub(receivedAt)
	if delta < 0 {
		delta = -delta
	}
	return delta <= v1MaxProtocolClockSkew
}
