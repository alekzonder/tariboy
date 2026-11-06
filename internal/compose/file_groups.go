package compose

import (
	"fmt"

	"github.com/alekzonder/tariboy/internal/agent"
)

type GroupSpec struct {
	Lead   string      `yaml:"lead"`
	Budget *BudgetSpec `yaml:"budget"`
}

type BudgetSpec struct {
	LimitUSD float64 `yaml:"limit_usd"`
	Period   string  `yaml:"period"`
	Mode     string  `yaml:"mode"`
}

// NormalizedMode maps the compose-facing mode onto the stored budget mode
// (warn|block). "enforce" is the spec's word for "block" (reject); "" defaults
// to warn.
func (b BudgetSpec) NormalizedMode() (string, error) {
	switch b.Mode {
	case "", "warn":
		return "warn", nil
	case "block", "enforce":
		return "block", nil
	default:
		return "", fmt.Errorf("invalid budget mode %q (want warn|block|enforce)", b.Mode)
	}
}

func (f File) validateGroup(name string, g GroupSpec) error {
	if !agent.ValidName(name) {
		return fmt.Errorf("invalid group name %q", name)
	}
	if g.Lead != "" {
		a, ok := f.Agents[g.Lead]
		if !ok || a.Group != name {
			return fmt.Errorf("group %q lead %q is not a member of that group", name, g.Lead)
		}
	}
	if g.Budget != nil {
		if _, err := g.Budget.NormalizedMode(); err != nil {
			return fmt.Errorf("group %q budget: %w", name, err)
		}
	}
	return nil
}
