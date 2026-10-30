package commands

import (
	"github.com/alekzonder/tariboy/internal/onboarding"
	"github.com/alekzonder/tariboy/internal/toolcheck"
)

// Response types of the onboarding.go commands, copied under an Onboarding
// prefix so their component names cannot collide with another domain's.

// OnboardingDefaultStore is the state of the default Store registration:
// registered, failed, removed or pending.
type OnboardingDefaultStore onboarding.DefaultStore

// OnboardingStarterImage is the state of the starter image: ready, building,
// failed, skipped (an image named basic already existed) or pending.
type OnboardingStarterImage onboarding.Starter

// OnboardingHarness reports whether one harness CLI is on the daemon PATH.
type OnboardingHarness toolcheck.Harness

// OnboardingStatus is the first-run state of the daemon (onboarding.status,
// onboarding.retry).
type OnboardingStatus struct {
	DefaultStore OnboardingDefaultStore `json:"default_store"`
	StarterImage OnboardingStarterImage `json:"starter_image"`
	Harnesses    []OnboardingHarness    `json:"harnesses"`
	Agents       int                    `json:"agents"`
}
