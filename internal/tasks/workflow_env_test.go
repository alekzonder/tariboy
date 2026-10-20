package tasks

import (
	"context"
	"reflect"
	"testing"
)

func TestQueueEnvSetListRemove(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	info, err := svc.SetQueueEnvInfo(ctx, actor, "dev", "LOG_LEVEL", "debug")
	if err != nil || info.Key != "LOG_LEVEL" || info.Value != "debug" || info.UpdatedAt == "" {
		t.Fatalf("set = %#v, %v", info, err)
	}
	if _, err := svc.SetQueueEnvInfo(ctx, actor, "DEV", "BASE_URL", "https://example.test"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetQueueSecret(ctx, actor, "DEV", "GH_TOKEN", "s3cr3t-value"); err != nil {
		t.Fatal(err)
	}
	items, err := svc.ListQueueEnv(ctx, actor, "DEV")
	if err != nil || len(items) != 2 || items[0].Key != "BASE_URL" || items[1] != info {
		t.Fatalf("env = %#v, %v", items, err)
	}
	// Env and secrets are listed apart.
	keys, err := svc.ListQueueSecretKeys(ctx, actor, "DEV")
	if err != nil || !reflect.DeepEqual(keys, []string{"GH_TOKEN"}) {
		t.Fatalf("secret keys = %v, %v", keys, err)
	}
	env, secrets, err := svc.queueEnvironment(ctx, "DEV")
	if err != nil ||
		!reflect.DeepEqual(env, map[string]string{"LOG_LEVEL": "debug", "BASE_URL": "https://example.test"}) ||
		!reflect.DeepEqual(secrets, map[string]string{"GH_TOKEN": "s3cr3t-value"}) {
		t.Fatalf("queueEnvironment = %v, %v, %v", env, secrets, err)
	}
	// A secret is not removed through the env and the other way round.
	if err := svc.RemoveQueueEnv(ctx, actor, "DEV", "GH_TOKEN"); ErrorCode(err) != "not_found" {
		t.Fatalf("env remove of a secret = %v; want not_found", err)
	}
	if err := svc.RemoveQueueSecret(ctx, actor, "DEV", "LOG_LEVEL"); ErrorCode(err) != "not_found" {
		t.Fatalf("secret remove of an env var = %v; want not_found", err)
	}
	if err := svc.RemoveQueueEnv(ctx, actor, "DEV", "LOG_LEVEL"); err != nil {
		t.Fatal(err)
	}
	if items, _ := svc.ListQueueEnv(ctx, actor, "DEV"); len(items) != 1 || items[0].Key != "BASE_URL" {
		t.Fatalf("env after remove = %#v", items)
	}
	if _, err := svc.ListQueueEnv(ctx, actor, "NOPE"); ErrorCode(err) != "not_found" {
		t.Fatalf("list of an unknown queue = %v", err)
	}
	if _, err := svc.SetQueueEnvInfo(ctx, actor, "DEV", "TARIBOY_X", "v"); ErrorCode(err) != "invalid_secret_key" {
		t.Fatalf("protocol key = %v", err)
	}
}

func TestQueueEnvKeyMovesBetweenEnvAndSecrets(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	if err := svc.SetQueueSecret(ctx, actor, "DEV", "API_URL", "old-secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetQueueEnvInfo(ctx, actor, "DEV", "API_URL", "https://api.test"); err != nil {
		t.Fatal(err)
	}
	if keys, _ := svc.ListQueueSecretKeys(ctx, actor, "DEV"); len(keys) != 0 {
		t.Fatalf("the key stayed a secret: %v", keys)
	}
	if items, _ := svc.ListQueueEnv(ctx, actor, "DEV"); len(items) != 1 || items[0].Value != "https://api.test" {
		t.Fatalf("env = %#v", items)
	}
	if err := svc.SetQueueSecret(ctx, actor, "DEV", "API_URL", "new-secret"); err != nil {
		t.Fatal(err)
	}
	if items, _ := svc.ListQueueEnv(ctx, actor, "DEV"); len(items) != 0 {
		t.Fatalf("the key stayed in env: %#v", items)
	}
	env, secrets, _ := svc.queueEnvironment(ctx, "DEV")
	if len(env) != 0 || secrets["API_URL"] != "new-secret" {
		t.Fatalf("env %v secrets %v", env, secrets)
	}
}

func TestQueueEnvIsCustomerOnlyAndAudited(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	agent := AgentActor("dev-1")
	if _, err := svc.SetQueueEnvInfo(ctx, agent, "DEV", "LOG_LEVEL", "x"); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent set = %v", err)
	}
	if _, err := svc.ListQueueEnv(ctx, agent, "DEV"); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent list = %v", err)
	}
	if err := svc.RemoveQueueEnv(ctx, agent, "DEV", "LOG_LEVEL"); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent remove = %v", err)
	}
	if _, err := svc.SetQueueEnvInfo(ctx, actor, "DEV", "LOG_LEVEL", "debug-value"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveQueueEnv(ctx, actor, "DEV", "LOG_LEVEL"); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, event := range queueEvents(t, svc, "DEV", "queue.env_") {
		kind, payload := event.Kind, eventJSON(t, event)
		if payload != `{"key":"LOG_LEVEL"}` {
			t.Fatalf("event %s payload %s; want the key only", kind, payload)
		}
		kinds = append(kinds, kind)
	}
	if !reflect.DeepEqual(kinds, []string{"queue.env_set", "queue.env_removed"}) {
		t.Fatalf("events = %v", kinds)
	}
}

func TestQueueEnvSatisfiesRequiredSecrets(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	digest := seedBindingImage(t, svc, "flow", "1", "developers")
	requireSecrets(t, svc, digest, "GH_TOKEN", "API_URL")
	mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	if err := svc.SetQueueSecret(ctx, actor, "DEV", "GH_TOKEN", "v"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetQueueEnvInfo(ctx, actor, "DEV", "API_URL", "https://api.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:1", 0); err != nil {
		t.Fatalf("bind with API_URL in env: %v", err)
	}
	if err := svc.RemoveQueueEnv(ctx, actor, "DEV", "API_URL"); ErrorCode(err) != "workflow_secret_missing" {
		t.Fatalf("remove of a required env var = %v", err)
	}
}
