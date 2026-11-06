package compose

// storesPhase is the first phase, where the stores section will be applied;
// it plans nothing yet.
type storesPhase struct{}

func (storesPhase) name() string                          { return "stores" }
func (storesPhase) plan(*planner, File, *liveState) error { return nil }
