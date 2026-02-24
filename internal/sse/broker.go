package sse

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/liangzd/hapi-lite/internal/session"
)

type Client struct {
	ID        string
	SessionID string // empty = subscribe all
	Events    chan string
}

type Broker struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

func NewBroker() *Broker {
	return &Broker{clients: make(map[string]*Client)}
}

func (b *Broker) Subscribe(c *Client) {
	b.mu.Lock()
	b.clients[c.ID] = c
	b.mu.Unlock()
}

func (b *Broker) Unsubscribe(id string) {
	b.mu.Lock()
	if c, ok := b.clients[id]; ok {
		close(c.Events)
		delete(b.clients, id)
	}
	b.mu.Unlock()
}

func (b *Broker) Publish(event session.SyncEvent) {
	data, _ := json.Marshal(event)
	msg := fmt.Sprintf("data: %s\n\n", data)

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, c := range b.clients {
		if c.SessionID == "" || c.SessionID == event.SessionID {
			select {
			case c.Events <- msg:
			default: // drop if full
			}
		}
	}
}
