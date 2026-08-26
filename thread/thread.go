// Package thread stores provider-neutral conversation history.
package thread

import (
	"errors"
	"sync"

	"github.com/carsonfeng/harness/model"
)

var (
	// ErrBusy indicates that another run currently owns the thread.
	ErrBusy = errors.New("thread: an active run already owns this thread")
)

// Thread stores one ordered conversation history. It is safe for concurrent access.
type Thread struct {
	mu       sync.RWMutex
	messages []model.Message
	runMu    sync.Mutex
}

// BeginRun acquires exclusive ownership for one agent run.
// @param none.
// @return release function or ownership error.
func (t *Thread) BeginRun() (func(), error) {
	if !t.runMu.TryLock() {
		return nil, ErrBusy
	}
	return t.runMu.Unlock, nil
}

// New creates a thread initialized with the supplied messages.
// @param messages initial messages.
// @return new thread.
func New(messages ...model.Message) *Thread {
	t := &Thread{}
	for _, message := range messages {
		t.Add(message)
	}
	return t
}

// Add appends a message after copying it.
// @param message message to append.
// @return none.
func (t *Thread) Add(message model.Message) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messages = append(t.messages, clone(message))
}

// Messages returns a snapshot that callers may safely modify.
// @param t conversation thread.
// @return copied messages.
func (t *Thread) Messages() []model.Message {
	t.mu.RLock()
	defer t.mu.RUnlock()
	messages := make([]model.Message, len(t.messages))
	for i, message := range t.messages {
		messages[i] = clone(message)
	}
	return messages
}

// clone deep-copies mutable message fields.
// @param message source message.
// @return independent message copy.
func clone(message model.Message) model.Message {
	message.ToolCalls = append([]model.ToolCall(nil), message.ToolCalls...)
	for i := range message.ToolCalls {
		message.ToolCalls[i].Arguments = append([]byte(nil), message.ToolCalls[i].Arguments...)
	}
	return message
}
