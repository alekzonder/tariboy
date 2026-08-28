package telegramplugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type AgentTopic struct {
	Name     string `json:"name"`
	ThreadID int64  `json:"thread_id"`
}

type ChatBinding struct {
	ChatID            int64                 `json:"chat_id"`
	ManagementTopicID int64                 `json:"management_topic_id"`
	AgentTopics       map[string]AgentTopic `json:"agent_topics"`
}

type State struct {
	Token             string                `json:"bot_token,omitempty"`
	AllowedUIDs       []int64               `json:"allowed_uids"`
	ChatID            int64                 `json:"chat_id,omitempty"`
	ManagementTopicID int64                 `json:"management_topic_id,omitempty"`
	Offset            int64                 `json:"offset,omitempty"`
	AgentTopics       map[string]AgentTopic `json:"agent_topics"`
	PendingChat       *ChatBinding          `json:"pending_chat,omitempty"`
}

type StateStore struct {
	mu    sync.Mutex
	path  string
	state State
}

func OpenState(workdir string) (*StateStore, error) {
	if err := os.MkdirAll(workdir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(workdir, 0o700); err != nil {
		return nil, err
	}
	store := &StateStore{path: filepath.Join(workdir, "telegram.json")}
	b, err := os.ReadFile(store.path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(b) > 0 {
		if err := os.Chmod(store.path, 0o600); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &store.state); err != nil {
			return nil, err
		}
	}
	if store.state.AllowedUIDs == nil {
		store.state.AllowedUIDs = []int64{}
	}
	if store.state.AgentTopics == nil {
		store.state.AgentTopics = map[string]AgentTopic{}
	}
	if store.state.PendingChat != nil && store.state.PendingChat.AgentTopics == nil {
		store.state.PendingChat.AgentTopics = map[string]AgentTopic{}
	}
	return store, nil
}

func (s *StateStore) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneState(s.state)
}

func (s *StateStore) Configure(token string, allowed []int64) error {
	return s.update(func(state *State) {
		if token != "" {
			state.Token = token
		}
		state.AllowedUIDs = normalizeUIDs(allowed)
	})
}

func (s *StateStore) BindChat(chatID, managementTopicID int64) error {
	return s.update(func(state *State) {
		if state.ChatID != chatID {
			state.AgentTopics = map[string]AgentTopic{}
		}
		state.ChatID = chatID
		state.ManagementTopicID = managementTopicID
		state.PendingChat = nil
	})
}

func (s *StateStore) BeginPendingChat(chatID, managementTopicID int64) error {
	return s.update(func(state *State) {
		if state.PendingChat == nil || state.PendingChat.ChatID != chatID {
			state.PendingChat = &ChatBinding{ChatID: chatID, ManagementTopicID: managementTopicID, AgentTopics: map[string]AgentTopic{}}
		}
	})
}

func (s *StateStore) SetPendingAgentTopic(chatID int64, id, name string, threadID int64) error {
	return s.update(func(state *State) {
		if state.PendingChat != nil && state.PendingChat.ChatID == chatID {
			state.PendingChat.AgentTopics[id] = AgentTopic{Name: name, ThreadID: threadID}
		}
	})
}

func (s *StateStore) CommitPendingChat(chatID int64) error {
	return s.update(func(state *State) {
		if state.PendingChat == nil || state.PendingChat.ChatID != chatID {
			return
		}
		state.ChatID = state.PendingChat.ChatID
		state.ManagementTopicID = state.PendingChat.ManagementTopicID
		state.AgentTopics = cloneTopics(state.PendingChat.AgentTopics)
		state.PendingChat = nil
	})
}

func (s *StateStore) SetAgentTopic(id, name string, threadID int64) error {
	return s.update(func(state *State) {
		state.AgentTopics[id] = AgentTopic{Name: name, ThreadID: threadID}
	})
}

func (s *StateStore) SetOffset(offset int64) error {
	return s.update(func(state *State) { state.Offset = offset })
}

func (s *StateStore) update(change func(*State)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	change(&s.state)
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func cloneState(state State) State {
	copy := state
	copy.AllowedUIDs = append([]int64(nil), state.AllowedUIDs...)
	copy.AgentTopics = cloneTopics(state.AgentTopics)
	if state.PendingChat != nil {
		pending := *state.PendingChat
		pending.AgentTopics = cloneTopics(state.PendingChat.AgentTopics)
		copy.PendingChat = &pending
	}
	return copy
}

func cloneTopics(topics map[string]AgentTopic) map[string]AgentTopic {
	copy := make(map[string]AgentTopic, len(topics))
	for id, topic := range topics {
		copy[id] = topic
	}
	return copy
}

func (s *StateStore) saveLocked() error {
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".telegram-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := json.NewEncoder(tmp).Encode(s.state); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

func normalizeUIDs(values []int64) []int64 {
	values = append([]int64(nil), values...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}
