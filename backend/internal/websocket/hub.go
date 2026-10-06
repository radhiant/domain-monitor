// Package websocket pushes live probe results to connected dashboards.
//
// The structure mirrors the server-monitor hub: one goroutine owns the client
// set, and slow clients are dropped rather than allowed to stall the probes.
package websocket

import (
	"sync"

	"github.com/rs/zerolog/log"

	"domain-monitor/backend/internal/model"
)

// Hub maintains the set of active clients and fans messages out to them.
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	broadcast  chan model.WSMessage
	register   chan *Client
	unregister chan *Client
	quit       chan struct{}

	// initial builds the payload a client receives the moment it connects, so
	// a display that reconnects mid-outage paints immediately instead of
	// waiting out a full probe cycle.
	initial func() any
}

// NewHub creates a hub. The initial function is called once per connection.
func NewHub(initial func() any) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan model.WSMessage, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		quit:       make(chan struct{}),
		initial:    initial,
	}
}

// Run executes the hub event loop.
func (h *Hub) Run() {
	for {
		select {
		case <-h.quit:
			h.mu.Lock()
			for client := range h.clients {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			count := len(h.clients)
			h.mu.Unlock()
			log.Info().Int("total_clients", count).Msg("dashboard connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Info().Int("total_clients", len(h.clients)).Msg("dashboard disconnected")
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// The client is not keeping up. Queue it for removal
					// instead of blocking every other display behind it.
					go func(c *Client) { h.unregister <- c }(client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to every connected dashboard.
func (h *Hub) Broadcast(msg model.WSMessage) {
	select {
	case h.broadcast <- msg:
	default:
		// The queue is full; dropping a frame is better than stalling a probe.
	}
}

// Clients reports how many dashboards are connected.
func (h *Hub) Clients() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Stop closes the hub and terminates all client connections.
func (h *Hub) Stop() {
	close(h.quit)
}
