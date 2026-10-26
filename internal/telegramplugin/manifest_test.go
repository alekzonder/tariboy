package telegramplugin

import "testing"

func TestManifestDeclaresTelegramOperatorAndMailSurfaces(t *testing.T) {
	manifest := Manifest("1.2.3")
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "telegram" || manifest.Exec != "tariboy-plugin-telegram" || len(manifest.OperatorCommands) != 3 {
		t.Fatalf("manifest = %+v", manifest)
	}
	if manifest.OperatorCommands[0].Path != "configure" || manifest.OperatorCommands[1].Path != "chat.setup" || manifest.OperatorCommands[2].Path != "status" {
		t.Fatalf("commands = %+v", manifest.OperatorCommands)
	}
	if !manifest.Mail.Send || !manifest.Mail.Receive || !manifest.HasType("mail-source") || !manifest.HasType("mail-sink") {
		t.Fatalf("mail = %+v types = %v", manifest.Mail, manifest.Types)
	}
	if manifest.Settings == nil || len(manifest.Settings.Sections) != 2 {
		t.Fatalf("settings = %+v", manifest.Settings)
	}
}
