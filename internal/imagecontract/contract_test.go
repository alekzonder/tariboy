package imagecontract

import (
	"strings"
	"testing"
)

func TestValidateNamesTheMissingImageCapability(t *testing.T) {
	valid := Input{
		Plugins:  []string{"whoami", "loop", "messages", "tasks"},
		Runtimes: []string{"identity", "messages", "user-prompt"},
		Skills: []Skill{
			{Name: "whoami"},
			{Name: "loop", Executables: map[string]bool{"scripts/loop.sh": true}},
			{Name: "messages"},
			{Name: "tasks", Executables: map[string]bool{"scripts/tasks.sh": true}},
		},
	}
	tests := []struct {
		name string
		edit func(*Input)
		want string
	}{
		{"unknown runtime", func(in *Input) { in.Runtimes = append(in.Runtimes, "missing-runtime") }, "missing-runtime"},
		{"unknown plugin", func(in *Input) { in.Plugins = append(in.Plugins, "missing-plugin") }, "missing-plugin"},
		{"runtime owner", func(in *Input) { in.Plugins = []string{"loop", "messages", "tasks"} }, "whoami"},
		{"packaged skill", func(in *Input) { in.Skills = in.Skills[1:] }, "whoami"},
		{"loop launcher", func(in *Input) { in.Skills[1].Executables = nil }, "scripts/loop.sh"},
		{"tasks launcher", func(in *Input) { in.Skills[3].Executables = nil }, "scripts/tasks.sh"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			input.Plugins = append([]string(nil), valid.Plugins...)
			input.Runtimes = append([]string(nil), valid.Runtimes...)
			input.Skills = append([]Skill(nil), valid.Skills...)
			tc.edit(&input)
			if err := Validate(input, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate error = %v, want name %q", err, tc.want)
			}
		})
	}
	if err := Validate(valid, nil); err != nil {
		t.Fatalf("valid image: %v", err)
	}
}

func TestImageContractAcceptsMessagesAlias(t *testing.T) {
	skills := []Skill{{Name: "messages"}, {Name: "mail"}}
	// An image built before the mailbox lists plugin messages and renders the
	// messages and awaiting-replies placeholders; it still provisions.
	if err := Validate(Input{Plugins: []string{"messages"}, Runtimes: []string{"messages", "awaiting-replies"}, Skills: skills}, nil); err != nil {
		t.Fatalf("old image with plugin messages: %v", err)
	}
	if err := Validate(Input{Plugins: []string{"mail"}, Runtimes: []string{"messages", "awaiting-replies"}, Skills: skills}, nil); err != nil {
		t.Fatalf("image with plugin mail: %v", err)
	}
	if err := Validate(Input{Runtimes: []string{"messages"}}, nil); err == nil {
		t.Fatal("the messages placeholder without mail or messages must be refused")
	}
	if err := Validate(Input{Plugins: []string{"mail"}}, nil); err == nil {
		t.Fatal("plugin mail without its packaged skill must be refused")
	}
}
