package realtime

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Event struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Topics   []string `json:"topics"`
	Reason   string   `json:"reason,omitempty"`
	EntityID uint     `json:"entity_id,omitempty"`
	At       string   `json:"at"`
}

type Client struct {
	id     uint64
	events chan Event
}

type Hub struct {
	mu      sync.RWMutex
	clients map[uint64]*Client
	nextID  uint64
	nextSeq uint64
}

var defaultHub = NewHub()

func NewHub() *Hub {
	return &Hub{clients: make(map[uint64]*Client)}
}

func Subscribe() (*Client, func()) {
	return defaultHub.Subscribe()
}

func Publish(topic string, reason string, entityID uint) {
	PublishTopics([]string{topic}, reason, entityID)
}

func PublishTopics(topics []string, reason string, entityID uint) {
	defaultHub.Publish(topics, reason, entityID)
}

func (h *Hub) Subscribe() (*Client, func()) {
	id := atomic.AddUint64(&h.nextID, 1)
	client := &Client{
		id:     id,
		events: make(chan Event, 32),
	}

	h.mu.Lock()
	h.clients[id] = client
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		if existing, ok := h.clients[id]; ok {
			delete(h.clients, id)
			close(existing.events)
		}
		h.mu.Unlock()
	}

	return client, unsubscribe
}

func (h *Hub) Publish(topics []string, reason string, entityID uint) {
	cleanTopics := normalizeTopics(topics)
	if len(cleanTopics) == 0 {
		return
	}

	seq := atomic.AddUint64(&h.nextSeq, 1)
	event := Event{
		ID:       fmt.Sprintf("%d", seq),
		Type:     "data_refresh",
		Topics:   cleanTopics,
		Reason:   reason,
		EntityID: entityID,
		At:       time.Now().UTC().Format(time.RFC3339Nano),
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, client := range h.clients {
		select {
		case client.events <- event:
		default:
			// Drop for slow clients. The browser will receive the next event or reconnect.
		}
	}
}

func (c *Client) Events() <-chan Event {
	return c.events
}

func MarshalEvent(event Event) string {
	payload, err := json.Marshal(event)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func normalizeTopics(topics []string) []string {
	seen := make(map[string]struct{}, len(topics))
	out := make([]string, 0, len(topics))
	for _, topic := range topics {
		if topic == "" {
			continue
		}
		if _, ok := seen[topic]; ok {
			continue
		}
		seen[topic] = struct{}{}
		out = append(out, topic)
	}
	return out
}
