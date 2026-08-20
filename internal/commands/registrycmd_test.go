package commands

import "testing"

func TestPushPullLoginRegisteredAndCLILocal(t *testing.T) {
	reg := BuildRegistry()
	for _, path := range []string{"push", "pull", "login"} {
		cmd, ok := reg.Get(path)
		if !ok {
			t.Fatalf("%s not registered", path)
		}
		if cmd.HTTP != nil {
			t.Fatalf("%s must be CLI-local (HTTP == nil)", path)
		}
		if cmd.Handler == nil {
			t.Fatalf("%s has no handler", path)
		}
	}
}
