package server

import (
	"sync"
	"time"
)

// Message is a single chat turn.
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Session holds the message history for one chat session.
type Session struct {
	ID       string
	messages []Message
	mu       sync.Mutex
}

func (s *Session) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
}

// Messages returns a snapshot of the session history.
func (s *Session) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, len(s.messages))
	copy(out, s.messages)
	return out
}

// ToAnthropicMessages converts the session history to Anthropic API message format.
// Only user and assistant turns with string content are included.
func (s *Session) ToAnthropicMessages() []AnthropicMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AnthropicMessage, 0, len(s.messages))
	for _, m := range s.messages {
		if m.Role == "user" || m.Role == "assistant" {
			out = append(out, AnthropicMessage{Role: m.Role, Content: m.Content})
		}
	}
	return out
}

// SessionStore manages active chat sessions.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*Session)}
}

func (s *SessionStore) Create(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := &Session{ID: id}
	s.sessions[id] = sess
	return sess
}

func (s *SessionStore) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	return ids
}
