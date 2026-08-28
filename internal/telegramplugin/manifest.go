package telegramplugin

import "github.com/alekzonder/tariboy/internal/plugins"

func Manifest(version string) plugins.Manifest {
	return plugins.Manifest{
		Name: "telegram", Version: version, ProtocolVersion: plugins.ProtocolVersion,
		Types: []string{"channel-source", "channel-sink"}, Exec: "tariboy-plugin-telegram",
		Description: "Telegram forum topics for Tariboy agents",
		Channels:    plugins.Channels{Publish: []string{"chat:telegram:*"}, Subscribe: []string{"chat:telegram:*"}},
		OperatorCommands: []plugins.OperatorCommand{
			{Path: "configure", Summary: "Configure the Telegram bot token and allowed user IDs", Action: "configure", Args: []plugins.OperatorArg{
				{Name: "token", Flag: "token-file", Type: "secret-file", Help: "owner-only token file, or - for stdin"},
				{Name: "allowed_uids", Flag: "allowed-uids", Type: "integer-list", Required: true, Help: "comma-separated Telegram user IDs; empty denies all"},
			}},
			{Path: "chat.setup", Summary: "Bind an existing forum supergroup and create Tariboy topics", Action: "chat_setup", Args: []plugins.OperatorArg{
				{Name: "chat_id", Flag: "chat-id", Type: "string", Required: true, Help: "Telegram supergroup chat ID"},
			}},
			{Path: "status", Summary: "Show Telegram integration status", Action: "status"},
		},
		Settings: &plugins.SettingsContribution{
			Title: "Telegram",
			Status: []plugins.SettingStatus{
				{Name: "token_configured", Label: "Token configured"},
				{Name: "allowlist_count", Label: "Allowed users"},
				{Name: "chat_id", Label: "Forum supergroup"},
				{Name: "management_topic_id", Label: "Tariboyd topic"},
			},
			Sections: []plugins.SettingSection{
				{Title: "Bot", Fields: []plugins.SettingField{
					{Name: "token", Label: "Bot token", Type: "password", Help: "Write-only; leave empty to keep the current token."},
					{Name: "allowed_uids", Label: "Allowed Telegram UIDs", Type: "integer-list", Required: true, Help: "Empty denies every incoming message."},
				}, Actions: []plugins.SettingAction{{Label: "Save bot settings", Action: "configure", Fields: []string{"token", "allowed_uids"}}}},
				{Title: "Forum supergroup", Fields: []plugins.SettingField{
					{Name: "chat_id", Label: "Chat ID", Type: "string", Required: true, Help: "Create the group and enable topics in Telegram first."},
				}, Actions: []plugins.SettingAction{{Label: "Set up chat", Action: "chat_setup", Fields: []string{"chat_id"}}}},
			},
		},
	}
}
