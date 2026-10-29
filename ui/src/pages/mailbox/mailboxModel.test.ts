import { describe, expect, it } from "vitest";
import type { MailUnread } from "@/lib/api";
import { letterTaskKey } from "@/lib/letters";
import { letter } from "@/test/fixtures/mailFixtures";
import {
  letterActions,
  letterState,
  marksReadOnOpen,
  principalName,
  questionCountsBySender,
} from "./mailboxModel";

describe("mailboxModel", () => {
  it("names principals without their prefix", () => {
    expect(principalName("agent:dev")).toBe("dev");
    expect(principalName("user:alice")).toBe("alice");
    expect(principalName("queue:dev,group:ops")).toBe("dev, ops");
  });

  it("reads the task key of a letter only when it is a string", () => {
    expect(letterTaskKey(letter({ data: { task_key: "DEV-1" } }))).toBe("DEV-1");
    expect(letterTaskKey(letter({ data: { task_key: 7 } }))).toBe("");
  });

  it("gives an open request Reply and Archive, a task question an answer form", () => {
    expect(letterActions(letter(), "inbox", "customer")).toEqual({
      reply: true,
      archive: true,
      requeue: false,
      answerInline: false,
      answerOnTask: false,
      taskLink: false,
    });
    const question = letter({ kind: "task.question", data: { task_key: "DEV-1" } });
    expect(letterActions(question, "inbox", "customer")).toMatchObject({
      reply: false,
      answerInline: true,
      answerOnTask: false,
      taskLink: true,
    });
    const linked = letter({ kind: "task.done", needs_reply: false, data: { task_key: "DEV-1" } });
    expect(letterActions(linked, "inbox", "customer")).toMatchObject({
      reply: false,
      taskLink: true,
    });
  });

  it("sends a question to its task when a comment cannot answer it", () => {
    const question = letter({ kind: "task.question", data: { task_key: "DEV-1" } });
    const onTask = { answerInline: false, answerOnTask: true, taskLink: false };
    expect(letterActions(question, "inbox", "agent")).toMatchObject(onTask);
    const workflow = letter({ ...question, from: "system:workflow" });
    expect(letterActions(workflow, "inbox", "customer")).toMatchObject(onTask);
    const closed = letter({ ...question, answered_at: "x" });
    expect(letterActions(closed, "archive", "customer")).toMatchObject({
      answerInline: false,
      answerOnTask: false,
      taskLink: true,
    });
    expect(letterActions(question, "sent", "customer").answerInline).toBe(false);
  });

  it("offers Requeue only on a dead letter in Archive", () => {
    const dead = letter({ dead_at: "2026-10-26T09:00:00Z", dead_reason: "queue_limit" });
    expect(letterActions(dead, "archive", "agent")).toMatchObject({ requeue: true, reply: false });
    expect(letterActions(letter({ answered_at: "x" }), "archive", "agent").requeue).toBe(false);
    expect(letterState(dead)).toBe("dead · queue_limit");
    expect(letterState(letter({ answered_at: "x", close_note: "read" }))).toBe("archived · read");
    expect(letterState(letter({ answered_at: "x", answer_id: "m2" }))).toBe("answered");
    expect(letterState(letter())).toBe("open");
  });

  it("marks an unread inbox letter read on open: a notice then moves to Archive, a request stays", () => {
    expect(marksReadOnOpen(letter({ needs_reply: false }), "inbox")).toBe(true);
    expect(marksReadOnOpen(letter(), "inbox")).toBe(true);
    expect(marksReadOnOpen(letter({ read_at: "x" }), "inbox")).toBe(false);
    expect(marksReadOnOpen(letter(), "sent")).toBe(false);
  });

  it("counts open task questions per asking agent", () => {
    const unread: MailUnread = {
      as: "user:customer",
      open: 5,
      unread: 2,
      by_sender: [
        { sender: "agent:dev", kind: "task.question", count: 2 },
        { sender: "agent:dev", kind: "message", count: 1 },
        { sender: "agent:ops", kind: "task.question", count: 1 },
        { sender: "user:bob", kind: "task.question", count: 1 },
      ],
    };
    expect(questionCountsBySender(unread)).toEqual(
      new Map([
        ["dev", 2],
        ["ops", 1],
      ]),
    );
  });
});
