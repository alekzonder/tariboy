package commands

import (
	"context"
	"net/http"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/onboarding"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/toolcheck"
)

var errNoOnboarding = api.UserError{Code: "no_onboarding", Msg: "onboarding is not available", Status: http.StatusServiceUnavailable}

func onboardingStatus() registry.Command {
	return registry.WithResult[OnboardingStatus](registry.Command{
		Path:    "onboarding.status",
		Summary: "Report the first-run state: default Store, starter image, harness CLIs and agent count",
		Args: []registry.Arg{
			{Name: "refresh", Flag: "refresh", Type: registry.Bool, Help: "look up the harness CLIs again instead of the 5-minute cache"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/onboarding"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			if c.Onboarding == nil {
				return nil, errNoOnboarding
			}
			refresh, _ := p.OptBool("refresh")
			return onboardingResult(c, c.Onboarding.Status(context.Background(), refresh))
		},
	})
}

func onboardingRetry() registry.Command {
	return registry.WithResult[OnboardingStatus](registry.Command{
		Path:    "onboarding.retry",
		Summary: "Register the default Store and build the starter image again when they are missing",
		HTTP:    &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/onboarding/retry"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			if c.Onboarding == nil {
				return nil, errNoOnboarding
			}
			return onboardingResult(c, c.Onboarding.Retry())
		},
	})
}

// onboardingSource is the default Store source this daemon registers.
func onboardingSource(c *registry.Ctx) string {
	if c.Onboarding == nil {
		return onboarding.DefaultStoreSource
	}
	return c.Onboarding.DefaultSource()
}

func onboardingResult(c *registry.Ctx, st onboarding.Status) (OnboardingStatus, error) {
	agents, err := agent.NewStore(c.Store).List()
	if err != nil {
		return OnboardingStatus{}, err
	}
	return OnboardingStatus{
		DefaultStore: OnboardingDefaultStore(st.DefaultStore),
		StarterImage: OnboardingStarterImage(st.StarterImage),
		Harnesses:    convertSlice(st.Harnesses, func(h toolcheck.Harness) OnboardingHarness { return OnboardingHarness(h) }),
		Agents:       len(agents),
	}, nil
}
