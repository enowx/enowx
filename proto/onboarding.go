package proto

import "time"

// Onboarding: the first run's record, kept with the user's synced settings, so that
// it's done once per account and only a device's own steps repeat on a new device.
const (
	MethodOnboardingGet = "onboarding.get" // OnboardingState
	MethodOnboardingSet = "onboarding.set" // OnboardingState → OnboardingState
)

// OnboardingState says when the user finished it, zero for not yet.
type OnboardingState struct {
	Done    time.Time `json:"done,omitzero"`
	Version int       `json:"version,omitempty"`
}
