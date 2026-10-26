package mailcli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/alekzonder/tariboy/internal/mail"
)

// printResult prints a result as JSON with --json, otherwise as compact text.
func printResult(parsed request, raw json.RawMessage, prog string, jsonOut bool, stdout, stderr io.Writer) int {
	if jsonOut {
		fmt.Fprintln(stdout, string(raw))
		return 0
	}
	var err error
	switch parsed.cmd.name {
	case "inbox", "sent", "archive":
		var list mail.MailList
		if err = json.Unmarshal(raw, &list); err == nil {
			printList(parsed, list, prog, stdout)
		}
	case "show":
		var thread mail.MailThread
		if err = json.Unmarshal(raw, &thread); err == nil {
			printThread(thread, stdout)
		}
	case "send":
		var result mail.MailSendResult
		if err = json.Unmarshal(raw, &result); err == nil {
			for _, l := range result.Letters {
				fmt.Fprintf(stdout, "sent %s to %s (%s)\n", l.ID, l.To, kindOf(l))
			}
		}
	case "reply":
		var result mail.MailReplyResult
		if err = json.Unmarshal(raw, &result); err == nil {
			fmt.Fprintf(stdout, "replied %s to %s (%s)\n", result.Letter.ID, result.Letter.To, kindOf(result.Letter))
			if result.Closed {
				fmt.Fprintf(stdout, "closed %s\n", parsed.pos["id"])
			}
		}
	case "close", "requeue":
		var result struct {
			Letter mail.Letter `json:"letter"`
		}
		if err = json.Unmarshal(raw, &result); err == nil {
			fmt.Fprintf(stdout, "%s %s: %s\n", map[string]string{"close": "closed", "requeue": "requeued"}[parsed.cmd.name], result.Letter.ID, state(result.Letter))
		}
	case "read":
		var result mail.MailReadResult
		if err = json.Unmarshal(raw, &result); err == nil {
			fmt.Fprintf(stdout, "read %d, closed notices %d\n", result.Read, result.ClosedNotices)
		}
	case "unread":
		var result mail.MailUnread
		if err = json.Unmarshal(raw, &result); err == nil {
			fmt.Fprintf(stdout, "%s: open %d, unread %d\n", result.As, result.Open, result.Unread)
			for _, s := range result.BySender {
				fmt.Fprintf(stdout, "  %-24s %-16s %d\n", s.Sender, s.Kind, s.Count)
			}
		}
	case "clear":
		var result mail.MailClearResult
		if err = json.Unmarshal(raw, &result); err == nil {
			fmt.Fprintf(stdout, "deleted %d open letters of %s\n", result.Deleted, parsed.flags["as"])
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// printList prints one line per letter, then the command of the next page.
func printList(parsed request, list mail.MailList, prog string, out io.Writer) {
	if len(list.Letters) == 0 {
		fmt.Fprintln(out, "no letters")
	}
	for _, l := range list.Letters {
		fmt.Fprintf(out, "%s  %s -> %s  %s  %s  %s\n", l.ID, l.From, l.To, kindOf(l), state(l), oneLine(l.Subject))
	}
	if list.NextBefore == "" {
		return
	}
	next := []string{prog, parsed.cmd.name}
	if parsed.on("all") {
		next = append(next, "--all")
	}
	for _, name := range []string{"limit", "as"} {
		if value, ok := parsed.flags[name]; ok {
			next = append(next, "--"+name, value)
		}
	}
	fmt.Fprintf(out, "next: %s --before %s\n", strings.Join(next, " "), list.NextBefore)
}

// printThread prints the headers of a letter, its body, then its reply.
func printThread(thread mail.MailThread, out io.Writer) {
	printLetter(thread.Letter, out)
	if r := thread.Reply; r != nil {
		fmt.Fprintf(out, "\n--- reply %s from %s at %s (%s)\n", r.ID, r.From, r.CreatedAt, kindOf(*r))
		fmt.Fprintln(out, strings.TrimRight(r.Body, "\n"))
	}
}

func printLetter(l mail.Letter, out io.Writer) {
	header := func(name, value string) {
		if value != "" {
			fmt.Fprintf(out, "%s: %s\n", name, value)
		}
	}
	header("id", l.ID)
	header("from", l.From)
	header("to", l.To)
	header("kind", kindOf(l))
	header("subject", l.Subject)
	header("state", state(l))
	header("created", l.CreatedAt)
	header("in_reply_to", l.InReplyTo)
	header("deadline", l.Deadline)
	header("close_note", l.CloseNote)
	if l.Attempts > 0 {
		header("attempts", fmt.Sprint(l.Attempts))
	}
	if len(l.Data) > 0 {
		data, _ := json.Marshal(l.Data)
		header("data", string(data))
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, strings.TrimRight(l.Body, "\n"))
}

// kindOf is the kind of a letter and whether it needs a reply.
func kindOf(l mail.Letter) string {
	if l.NeedsReply {
		return l.Kind + " request"
	}
	return l.Kind + " notice"
}

// state is open, answered, closed (without a reply) or dead:<reason>.
func state(l mail.Letter) string {
	switch {
	case l.DeadAt != "":
		return "dead:" + l.DeadReason
	case l.AnsweredAt != "" && l.AnswerID != "":
		return "answered"
	case l.AnsweredAt != "":
		return "closed"
	}
	return "open"
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
