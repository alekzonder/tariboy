package agent

import (
	"reflect"
	"testing"

	"github.com/alekzonder/tariboy/internal/store/storetest"
)

func TestPoolMembershipsListsQueuePools(t *testing.T) {
	db := storetest.Open(t)
	s := NewStore(db)
	for _, name := range []string{"dev", "other"} {
		if err := s.Create(Agent{Name: name, ImageRef: "basic:latest"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.PoolMemberships("dev")
	if err != nil || len(got) != 0 {
		t.Fatalf("memberships before pools = %+v, %v", got, err)
	}
	for _, stmt := range []string{
		`INSERT INTO task_queues(prefix,name,created_at,updated_at) VALUES ('DAO','DAO','t','t'),('DEV','DEV','t','t')`,
		`INSERT INTO task_agent_pools(id,queue_prefix,name,created_at,updated_at) VALUES (1,'DEV','reviewers','t','t'),(2,'DAO','developers','t','t'),(3,'DAO','others','t','t')`,
		`INSERT INTO task_agent_pool_members(pool_id,agent,position) VALUES (1,'dev',0),(2,'dev',0),(3,'other',0)`,
	} {
		if _, err := db.DB.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	got, err = s.PoolMemberships("dev")
	if err != nil {
		t.Fatal(err)
	}
	want := []PoolMembership{{Queue: "DAO", Pool: "developers"}, {Queue: "DEV", Pool: "reviewers"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("memberships = %+v, want %+v", got, want)
	}
}
