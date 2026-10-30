package mailcli

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

const operatorOnlyNote = " Operator-only: agent mode cannot run it."

var helpText = map[string]commandHelp{
	"inbox": {"List the open letters in the mailbox, oldest first", "inbox [--all] [--limit N] [--before CURSOR]",
		"Lists the letters addressed to you that are still open, oldest first: one line per letter with id, sender -> recipient, kind (request or notice), state, and subject. --all adds the closed letters. " +
			"When more remain, a last line next: prints the command for the next page. Operator mode reads the customer's mailbox unless --as names another principal.",
		"", "tmail inbox\ntmail inbox --all --limit 20"},
	"sent": {"List the letters you sent, newest first", "sent [--limit N] [--before CURSOR]",
		"Lists the letters you sent with their state: open, answered, closed, or dead:REASON. Operator mode reads the customer's letters unless --as names another principal.",
		"", "tmail sent\ntmail sent --limit 10"},
	"archive": {"List the closed letters, newest first, dead ones tagged", "archive [--limit N] [--before CURSOR]",
		"Lists the letters addressed to you that are closed: answered, archived without a reply, read notices, and dead letters, tagged dead:REASON. Requeue a dead letter with requeue.",
		"", "tmail archive\ntmail archive --limit 20 --before 2026-10-26T10:00:00.000000000Z/42"},
	"show": {"Show one letter and its reply, if any", "show ID",
		"Prints the headers of the letter, its body, then the reply that closed it, if any.",
		"ID  Letter id (required)", "tmail show m-20261026T104200Z-000007"},
	"send": {"Send a letter: a request unless --notice", "send --to PRINCIPAL[,PRINCIPAL] --subject TEXT --text TEXT [flags]",
		"Sends one letter to each recipient. The sender is you: the calling agent in agent mode, the customer in operator mode unless --as is given. " +
			"A letter is a request that the recipient must answer unless --notice makes it a notice. --text - reads the body from stdin; prefer it for long or multi-line text. " +
			"--deadline sets when you expect a reply. Prints the id of each letter written.",
		"", "tmail send --to user:customer --subject 'Question about DEV-12' --text 'Which bucket should I use?'\n" +
			"tmail send --to agent:reviewer,agent:qa --subject 'Build ready' --text 'Build 42 is green.' --notice\n" +
			"printf '%s\\n' \"$REPORT\" | tmail send --to agent:lead --subject Report --kind report --text -"},
	"reply": {"Reply to a letter, which closes it", "reply ID TEXT [--ask]",
		"Answers a letter addressed to you and closes it. The reply goes to the sender and is a notice unless --ask makes it a request that needs an answer. " +
			"TEXT may be given with --text instead; a TEXT of - reads it from stdin. In operator mode, --as agent:NAME replies on the agent's behalf.",
		"ID  Letter id (required)\nTEXT  Reply text, or - for stdin (required unless --text is given)",
		"tmail reply m-20261026T104200Z-000007 'Done: the bucket is created.'\ntmail reply m-20261026T104200Z-000007 'Which region?' --ask"},
	"requeue": {"Return one of your dead letters to the inbox", "requeue ID",
		"A request nobody answered in time is dead-lettered. Requeue puts it back in your inbox as an open letter.",
		"ID  Letter id (required)", "tmail requeue m-20261026T104200Z-000007"},
	"close": {"Archive a letter without a reply (operator-only)", "close ID [--note TEXT]",
		"Closes a letter in the mailbox without answering it and keeps the note with it." + operatorOnlyNote + " Agents answer a letter with reply.",
		"ID  Letter id (required)", "tmail close m-20261026T104200Z-000007 --note 'answered in chat'"},
	"read": {"Mark letters read (operator-only)", "read ID...",
		"Marks the letters read. A read notice to an agent closes; a notice to the customer stays open until it is archived." + operatorOnlyNote,
		"ID  Letter ids (at least one)", "tmail read m-20261026T104200Z-000007"},
	"unread": {"Count the open letters by sender and kind (operator-only)", "unread",
		"Prints the open and unread counts of the mailbox and the open letters per sender and kind." + operatorOnlyNote,
		"", "tmail unread\ntmail unread --as agent:worker"},
	"clear": {"Delete the open letters in an agent's inbox (operator-only)", "clear --as agent:NAME [--yes]",
		"Deletes every open letter addressed to the agent. Asks for confirmation on stdin unless --yes is given." + operatorOnlyNote,
		"", "tmail clear --as agent:worker\ntmail clear --as agent:worker --yes"},
}

const globalHelp = `Global flags:
  --help, -h   Show local help for a command; never execute it
  --help-json  Print the complete command tree as JSON (root only)
  --json       Print command results as JSON
  --version    Print the client build version (root only)`

// runTextHelp prints help before parsing or socket selection, in both modes.
func runTextHelp(args []string, prog string, stdout, stderr io.Writer) (int, bool) {
	explicit := len(args) == 0 || slices.Contains(args, "--help") || slices.Contains(args, "-h")
	if len(args) > 0 && args[0] == "help" {
		explicit, args = true, args[1:]
	}
	if !explicit {
		return 0, false
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printRootHelp(prog, stdout)
		return 0, true
	}
	cmd, ok := findCommand(args[0])
	if !ok {
		fmt.Fprintf(stderr, "mail: unknown help command %q\n", args[0])
		return 2, true
	}
	printCommandHelp(cmd, prog, stdout)
	return 0, true
}

func printRootHelp(prog string, out io.Writer) {
	if prog == "tmail" {
		fmt.Fprintln(out, "Usage: tmail <command> [args] [flags]\n\nMail client (tariboy-mail; tmail is its alias).\nWith non-empty TARIBOY_TOOLS_SOCKET: identity-bound agent mode; you are the calling agent.\nOtherwise: operator mode on the host Unix daemon socket, acting as the customer unless --as is given.\nHelp is local and lists both modes; execution still enforces their permissions.")
	} else {
		fmt.Fprintf(out, "Usage: %s <command> [args] [flags]\n\nMail client: the commands, flags, and output of tmail.\nAlways operator mode on the host Unix daemon socket, also for an agent:\nit acts as the customer unless --as names another principal.\n", prog)
	}
	fmt.Fprintln(out, "\nCommands:")
	for _, c := range commands {
		fmt.Fprintf(out, "  %-10s %s\n", c.name, c.help.summary)
	}
	fmt.Fprintf(out, "\nExamples:\n  %s inbox\n  %s reply --help\n\n%s\n", prog, prog, globalHelp)
}

func printCommandHelp(c command, prog string, out io.Writer) {
	var text strings.Builder
	fmt.Fprintf(&text, "Usage: tmail %s\n\n%s\n\n%s\n", c.help.usage, c.help.summary, c.help.help)
	if arguments := commandArguments(c); arguments != "" {
		fmt.Fprintf(&text, "\nArguments and flags:\n%s\n", arguments)
	}
	fmt.Fprintf(&text, "\nExamples:\n%s\n\n%s\n", c.help.examples, globalHelp)
	// The help texts name tmail in usage lines and examples.
	fmt.Fprint(out, strings.ReplaceAll(text.String(), "tmail ", prog+" "))
}

func commandArguments(c command) string {
	lines := []string{}
	if c.help.arguments != "" {
		lines = append(lines, c.help.arguments)
	}
	for _, f := range sortedFlags(c) {
		value := " VALUE"
		if f.boolean {
			value = ""
		}
		lines = append(lines, fmt.Sprintf("--%s%s  %s", f.name, value, f.help))
	}
	return strings.Join(lines, "\n")
}

func sortedFlags(c command) []flagSpec {
	flags := slices.Clone(c.flags)
	sort.Slice(flags, func(i, j int) bool { return flags[i].name < flags[j].name })
	return flags
}

// runHelpJSON prints every command with its help, as ttasks --help-json does.
func runHelpJSON(stdout, stderr io.Writer) int {
	tree := map[string]any{}
	for _, c := range commands {
		names := []string{}
		descriptions := map[string]string{}
		for _, f := range sortedFlags(c) {
			names = append(names, f.name)
			descriptions[f.name] = f.help
		}
		tree[c.name] = map[string]any{
			"summary": c.help.summary, "usage": c.help.usage, "help": c.help.help,
			"arguments": c.help.arguments, "examples": strings.Split(c.help.examples, "\n"),
			"flags": names, "flag_help": descriptions, "operator_only": c.operatorOnly,
		}
	}
	encoded, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(encoded))
	return 0
}
