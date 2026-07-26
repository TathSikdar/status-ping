package monitor

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// Hub fans out snapshots to all connected dashboards. In-memory: fine for a
// single backend instance.
// ponytail: in-memory fanout, switch to Redis pub/sub only if you run >1 backend.
type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]chan []byte
	last    []byte // latest snapshot, sent to new clients immediately
}

func NewHub() *Hub { return &Hub{clients: map[*websocket.Conn]chan []byte{}} }

func (h *Hub) Broadcast(msg []byte) {
	h.mu.Lock()
	h.last = msg
	for _, send := range h.clients {
		select {
		case send <- msg:
		default: // slow client, drop this frame
		}
	}
	h.mu.Unlock()
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	send := make(chan []byte, 8)
	h.mu.Lock()
	h.clients[conn] = send
	last := h.last
	h.mu.Unlock()

	if last != nil {
		conn.WriteMessage(websocket.TextMessage, last)
	}

	// writer goroutine
	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
			conn.Close()
		}()
		for msg := range send {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()

	// reader: we ignore inbound messages but must drain to detect close
	go func() {
		defer close(send)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	log.Printf("ws client connected (%d total)", len(h.clients))
}
