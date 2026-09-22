package bus

import "testing"

// The queue is the messages this participant has not answered: another
// participant said them, and no reply of this participant points at them.
func TestUnansweredExcludesRepliedAndOwnMessages(t *testing.T) {
	b, chat := chatFixture(t)
	first := publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "one"})
	second := publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "two"})
	publish(t, b, Message{Channel: chat.Channel, Source: "agent:worker", Type: "message", Text: "my own message"})
	if _, err := b.Reply("worker", first.ID, "answered one", nil, ""); err != nil {
		t.Fatal(err)
	}
	queue, err := b.Unanswered(chat.ID, "agent:worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].ID != second.ID {
		t.Fatalf("only the unreplied message belongs in the queue: %+v", queue)
	}
}

// Review Focus item 4: a plain send fills no in_reply_to, so it answers
// nothing. Pinned so a later helpful fallback cannot quietly retire a question.
func TestPlainSendDoesNotCountAsAnAnswer(t *testing.T) {
	b, chat := chatFixture(t)
	orig := publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "question"})
	publish(t, b, Message{Channel: chat.Channel, Source: "agent:worker", Type: "message", Text: "unrelated"})
	queue, err := b.Unanswered(chat.ID, "agent:worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].ID != orig.ID {
		t.Fatalf("a plain send must not retire the question: %+v", queue)
	}
}

// A principal that joins later owes no answer for what was said before it was
// in the chat.
func TestUnansweredIgnoresMessagesOlderThanJoin(t *testing.T) {
	b, chat := chatFixture(t)
	publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "before reviewer joined"})
	if err := b.AddParticipant(chat.ID, "agent:reviewer", "member"); err != nil {
		t.Fatal(err)
	}
	queue, err := b.Unanswered(chat.ID, "agent:reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 0 {
		t.Fatalf("a new participant inherits no backlog: %+v", queue)
	}
	asked := publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "and now?"})
	queue, err = b.Unanswered(chat.ID, "agent:reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].ID != asked.ID {
		t.Fatalf("a message sent after the join is owed an answer: %+v", queue)
	}
}

// A principal that is not in the chat owes nothing, and asking about a chat
// that does not exist is an empty queue rather than an error the caller has to
// special-case.
func TestUnansweredForNonParticipantIsEmpty(t *testing.T) {
	b, chat := chatFixture(t)
	publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "hello"})
	queue, err := b.Unanswered(chat.ID, "agent:stranger")
	if err != nil || len(queue) != 0 {
		t.Fatalf("queue = %+v err = %v, want empty", queue, err)
	}
	queue, err = b.Unanswered("dm:nobody", "agent:worker")
	if err != nil || len(queue) != 0 {
		t.Fatalf("queue = %+v err = %v, want empty", queue, err)
	}
}

// The agent-wide queue is every chat it takes part in, so one call is the whole
// answering obligation of an iteration.
func TestUnansweredForPrincipalSpansChats(t *testing.T) {
	b, chat := chatFixture(t)
	tasksChat, err := b.GetChat(ChatIDTasks("worker"))
	if err != nil {
		t.Fatal(err)
	}
	inDM := publish(t, b, Message{Channel: chat.Channel, Source: "user:customer", Type: "message", Text: "in the conversation"})
	inTasks := publish(t, b, Message{Channel: tasksChat.Channel, Source: "user:customer", Type: "task.question", Text: "on the task"})
	queues, err := b.UnansweredForPrincipal("agent:worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(queues[chat.ID]) != 1 || queues[chat.ID][0].ID != inDM.ID {
		t.Fatalf("conversation queue = %+v", queues[chat.ID])
	}
	if len(queues[tasksChat.ID]) != 1 || queues[tasksChat.ID][0].ID != inTasks.ID {
		t.Fatalf("tasks queue = %+v", queues[tasksChat.ID])
	}
	if _, present := queues[ChatIDService("worker")]; present {
		t.Fatalf("a chat with nothing unanswered must not appear: %v", queues)
	}
}
