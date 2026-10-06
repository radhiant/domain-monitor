package websocket

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"domain-monitor/backend/internal/model"
)

const (
	// writeWait is how long a single write may take.
	writeWait = 10 * time.Second

	// pongWait is how long we wait for a pong before assuming the display is
	// gone. A TV browser that goes to sleep is detected here.
	pongWait = 60 * time.Second

	// pingPeriod must stay below pongWait.
	pingPeriod = (pongWait * 9) / 10

	// maxMessageSize bounds what a client may send; it only ever sends pings.
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 8192,
	CheckOrigin: func(r *http.Request) bool {
		return true // Origin is enforced by the CORS middleware in the router.
	},
}

// Client bridges one websocket connection and the hub.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan model.WSMessage
}

// NewClient constructs a client bound to a hub.
func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:  hub,
		conn: conn,
		// The buffer is generous because a full snapshot is large and a
		// reconnecting display receives one immediately.
		send: make(chan model.WSMessage, 64),
	}
}

// readPump consumes client frames. The dashboard only ever sends keepalives,
// but the loop must run for close and pong handling to work.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Warn().Err(err).Msg("unexpected websocket close")
			}
			return
		}

		var incoming struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(message, &incoming); err == nil && incoming.Type == "ping" {
			select {
			case c.send <- model.WSMessage{Type: "pong", Timestamp: time.Now().Unix()}:
			default:
			}
		}
	}
}

// writePump serialises hub messages onto the connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if data, err := json.Marshal(message); err == nil {
				_, _ = w.Write(data)
			}
			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeWs upgrades a request and seeds the new client with a full snapshot.
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to upgrade websocket connection")
		return
	}

	client := NewClient(hub, conn)

	if hub.initial != nil {
		client.send <- model.WSMessage{
			Type:      "snapshot",
			Timestamp: time.Now().Unix(),
			Data:      hub.initial(),
		}
	}

	hub.register <- client

	go client.writePump()
	go client.readPump()
}
