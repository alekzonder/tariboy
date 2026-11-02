package agent

import (
	"reflect"
	"testing"

	"github.com/alekzonder/tariboy/internal/store/storetest"
)

func TestCreateStoresMailInstantSenders(t *testing.T) {
	s := NewStore(storetest.Open(t))
	for name, senders := range map[string][]string{"set": {"agent:lead"}, "empty": {}, "unset": nil} {
		if err := s.Create(Agent{Name: name, ImageRef: "i:latest", MailInstantSenders: senders}); err != nil {
			t.Fatal(err)
		}
		got, err := s.Get(name)
		if err != nil || !reflect.DeepEqual(got.MailInstantSenders, senders) {
			t.Fatalf("%s: senders = %#v, %v; want %#v", name, got.MailInstantSenders, err, senders)
		}
	}
}
