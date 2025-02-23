package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSServer struct {
	port string

	// Client management
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex

	// Shutdown
	shutdown chan struct{}
}

func NewWSServer(port string) *WSServer {
	return &WSServer{
		port:     port,
		clients:  make(map[*websocket.Conn]bool),
		shutdown: make(chan struct{}),
	}
}

func (ws *WSServer) handleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	// Register client
	ws.mu.Lock()
	ws.clients[conn] = true
	clientCount := len(ws.clients)
	ws.mu.Unlock()

	logger.WithFields(logrus.Fields{
		"total_clients": clientCount,
		"remote_addr":   r.RemoteAddr,
	}).Info("🔌 New WebSocket client connected")

	// Handle client disconnection
	defer func() {
		ws.mu.Lock()
		delete(ws.clients, conn)
		clientCount := len(ws.clients)
		ws.mu.Unlock()

		// 🔹 Log client disconnection
		logger.WithFields(logrus.Fields{
			"total_clients": clientCount,
			"remote_addr":   r.RemoteAddr,
		}).Info("WebSocket client disconnected")
	}()

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (ws *WSServer) BroadcastUpdate(update WSResponse) {
	data, err := json.Marshal(update)
	if err != nil {
		log.Printf("Failed to marshal update: %v", err)
		return
	}

	ws.mu.RLock()
	for client := range ws.clients {
		err := client.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			log.Printf("Failed to send message to client: %v", err)
			client.Close()
			ws.mu.RUnlock()
			ws.mu.Lock()
			delete(ws.clients, client)
			ws.mu.Unlock()
			ws.mu.RLock()
		}
	}
	ws.mu.RUnlock()
}

func (ws *WSServer) Start() error {
	// Set up WebSocket endpoint
	http.HandleFunc("/ws", ws.handleConnection)

	// Start HTTP server
	log.Printf("🌍 WebSocket server starting on %s", ws.port)
	go func() {
		if err := http.ListenAndServe(ws.port, nil); err != nil {
			log.Printf("WebSocket server error: %v", err)
		}
	}()

	return nil
}

func (ws *WSServer) Stop() {
	// Close all client connections
	ws.mu.Lock()
	for client := range ws.clients {
		client.Close()
		delete(ws.clients, client)
	}
	ws.mu.Unlock()

	close(ws.shutdown)
}
