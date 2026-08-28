package telegramplugin

import "testing"

func TestManifestDeclaresTelegramOperatorAndChannelSurfaces(t *testing.T) {
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
	if len(manifest.Channels.Publish) != 1 || manifest.Channels.Publish[0] != "chat:telegram:*" || manifest.Channels.Subscribe[0] != "chat:telegram:*" {
		t.Fatalf("channels = %+v", manifest.Channels)
	}
	if manifest.Settings == nil || len(manifest.Settings.Sections) != 2 {
		t.Fatalf("settings = %+v", manifest.Settings)
	}
}
