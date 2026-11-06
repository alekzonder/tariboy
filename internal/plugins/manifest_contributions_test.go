package plugins

import "testing"

// contributionStep is one violation of a chain: validateContributions must
// report want, and fix then removes that violation.
type contributionStep struct {
	want string
	fix  func()
}

func runContributionChain(t *testing.T, m *Manifest, steps []contributionStep) {
	t.Helper()
	for _, step := range steps {
		err := m.validateContributions()
		if err == nil {
			t.Fatalf("validateContributions() = nil, want %q", step.want)
		}
		if err.Error() != step.want {
			t.Fatalf("validateContributions() = %q, want %q", err, step.want)
		}
		step.fix()
	}
	if err := m.validateContributions(); err != nil {
		t.Fatalf("validateContributions() after every fix = %v", err)
	}
}

func TestValidateOperatorCommandChecksInOrder(t *testing.T) {
	t.Parallel()
	m := Manifest{Name: "p", OperatorCommands: []OperatorCommand{
		{Path: "", Action: "Bad Action!"},
		{Path: "fine", Summary: "dup", Action: "dup"},
	}, Settings: &SettingsContribution{}}
	first := &m.OperatorCommands[0]
	runContributionChain(t, &m, []contributionStep{
		{`plugin p has invalid relative operator command path ""`, func() { first.Path = "p.configure" }},
		{`plugin p has invalid relative operator command path "p.configure"`, func() { first.Path = "ok.Bad Part!" }},
		{`plugin p has invalid operator command path "ok.Bad Part!"`, func() { first.Path = "fine" }},
		{`plugin p operator command "fine" has no summary`, func() { first.Summary = "s" }},
		{`plugin p operator command "fine" has invalid action "Bad Action!"`, func() { first.Action = "act" }},
		{`plugin p declares operator command "fine" more than once`, func() { first.Path = "first" }},
		{`plugin p settings title is required`, func() { m.Settings = nil }},
	})
}

func TestValidateOperatorArgChecksInOrder(t *testing.T) {
	t.Parallel()
	args := []OperatorArg{
		{Name: "Bad Name!", Type: "string"},
		{Name: "typed", Type: "json"},
		{Name: "same", Type: "string"},
		{Name: "same", Type: "string"},
		{Name: "flagged", Type: "string", Flag: "Bad Flag!"},
		{Name: "secret", Type: "secret-file"},
	}
	m := Manifest{Name: "p", OperatorCommands: []OperatorCommand{{Path: "c", Summary: "s", Action: "a", Args: args}}}
	runContributionChain(t, &m, []contributionStep{
		{`plugin p operator command "c" has invalid argument "Bad Name!"`, func() { args[0].Name = "named" }},
		{`plugin p operator command "c" has invalid argument "typed"`, func() { args[1].Type = "boolean" }},
		{`plugin p operator command "c" repeats argument "same"`, func() { args[3].Name = "other" }},
		{`plugin p operator command "c" has invalid or duplicate flag "Bad Flag!"`, func() { args[4].Flag = "shared" }},
		{`plugin p operator command "c" secret argument "secret" must be a flag`, func() { args[5].Flag = "shared" }},
		{`plugin p operator command "c" has invalid or duplicate flag "shared"`, func() { args[5].Flag = "secret-file" }},
	})
}

func TestValidateSettingsChecksInOrder(t *testing.T) {
	t.Parallel()
	settings := &SettingsContribution{
		Status: []SettingStatus{{Name: "Bad Name!", Label: "l"}, {Name: "up"}, {Name: "twice", Label: "l"}, {Name: "twice", Label: "l"}},
		Sections: []SettingSection{{
			Fields: []SettingField{
				{Name: "Bad Name!", Label: "l", Type: "string"},
				{Name: "unlabelled", Type: "string"},
				{Name: "typed", Label: "l", Type: "html"},
				{Name: "first", Label: "l", Type: "password"},
			},
			Actions: []SettingAction{
				{Action: "unlabelled"},
				{Label: "l", Action: "Bad Action!"},
				{Label: "l", Action: "save", Fields: []string{"first", "later"}},
			},
		}, {
			Title:  "Second",
			Fields: []SettingField{{Name: "first", Label: "l", Type: "string"}},
		}},
	}
	section := &settings.Sections[0]
	m := Manifest{Name: "p", Settings: settings}
	runContributionChain(t, &m, []contributionStep{
		{`plugin p settings title is required`, func() { settings.Title = "T" }},
		{`plugin p has invalid or duplicate settings status "Bad Name!"`, func() { settings.Status[0].Name = "named" }},
		{`plugin p has invalid or duplicate settings status "up"`, func() { settings.Status[1].Label = "l" }},
		{`plugin p has invalid or duplicate settings status "twice"`, func() { settings.Status[3].Name = "again" }},
		{`plugin p settings section title is required`, func() { section.Title = "First" }},
		{`plugin p has invalid or duplicate settings field "Bad Name!"`, func() { section.Fields[0].Name = "named" }},
		{`plugin p has invalid or duplicate settings field "unlabelled"`, func() { section.Fields[1].Label = "l" }},
		{`plugin p has invalid or duplicate settings field "typed"`, func() { section.Fields[2].Type = "integer-list" }},
		{`plugin p has invalid settings action "unlabelled"`, func() { section.Actions[0].Label = "l" }},
		{`plugin p has invalid settings action "Bad Action!"`, func() { section.Actions[1].Action = "ok" }},
		{`plugin p settings action "save" references unknown field "later"`, func() { section.Actions[2].Fields = []string{"first"} }},
		{`plugin p has invalid or duplicate settings field "first"`, func() { settings.Sections[1].Fields[0].Name = "second" }},
	})
}
