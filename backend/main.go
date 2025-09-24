package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// --- Structs cho WebSocket ---
type Client struct {
	ID   string
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

type Hub struct {
	clients        map[string]*Client
	register       chan *Client
	unregister     chan *Client
	gameState      GameState
	gameStateMutex sync.RWMutex
}

// --- Structs cho Game ---
type Card struct {
	Suit string `json:"suit"`
	Rank string `json:"rank"`
}

type Player struct {
	ID      string `json:"id"`
	IsReady bool   `json:"isReady"`
	Hand    []Card `json:"hand"`
}

type GameState struct {
	Players     map[string]Player `json:"players"`
	GameStarted bool              `json:"gameStarted"`
	Deck        []Card            `json:"-"`
}

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// --- Khởi tạo Hub ---
func newHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		gameState:  GameState{Players: make(map[string]Player)},
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client.ID] = client
			h.addPlayer(client.ID)
			log.Printf("Client %s registered. Total: %d", client.ID, len(h.clients))

		case client := <-h.unregister:
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(client.send)
				h.removePlayer(client.ID)
				log.Printf("Client %s unregistered. Total: %d", client.ID, len(h.clients))
			}
		}
	}
}

// --- Logic Game ---

func (h *Hub) addPlayer(playerID string) {
	h.gameStateMutex.Lock()
	h.gameState.Players[playerID] = Player{ID: playerID, Hand: []Card{}}
	h.gameStateMutex.Unlock()
	h.broadcastGameState()
}

func (h *Hub) removePlayer(playerID string) {
	h.gameStateMutex.Lock()
	delete(h.gameState.Players, playerID)
	if len(h.gameState.Players) < 2 && h.gameState.GameStarted {
		h.gameState.GameStarted = false
		for id, p := range h.gameState.Players {
			p.IsReady = false
			h.gameState.Players[id] = p
		}
	}
	h.gameStateMutex.Unlock()
	h.broadcastGameState()
}

func (h *Hub) handlePlayerReady(playerID string, isReady bool) {
	h.gameStateMutex.Lock()
	if player, ok := h.gameState.Players[playerID]; ok {
		player.IsReady = isReady
		h.gameState.Players[playerID] = player
	}
	if len(h.gameState.Players) >= 2 && !h.gameState.GameStarted {
		allReady := true
		for _, p := range h.gameState.Players {
			if !p.IsReady {
				allReady = false
				break
			}
		}
		if allReady {
			h.startGameUnsafe()
		}
	}
	h.gameStateMutex.Unlock()
	h.broadcastGameState()
}

func (h *Hub) startGameUnsafe() {
	log.Println("--- GAME STARTING ---")
	suits := []string{"♥", "♦", "♣", "♠"}
	ranks := []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A"}
	deck := make([]Card, 0, 52)
	for _, s := range suits {
		for _, r := range ranks {
			deck = append(deck, Card{Suit: s, Rank: r})
		}
	}
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	h.gameState.Deck = deck
	h.gameState.GameStarted = true

	for id, p := range h.gameState.Players {
		p.Hand, p.IsReady = []Card{}, false
		if len(h.gameState.Deck) > 1 {
			p.Hand = append(p.Hand, h.gameState.Deck[0], h.gameState.Deck[1])
			h.gameState.Deck = h.gameState.Deck[2:]
		}
		h.gameState.Players[id] = p
		log.Printf("Dealt to %s", id)
	}
}

func (h *Hub) broadcastGameState() {
	h.gameStateMutex.RLock()
	payload, err := json.Marshal(h.gameState)
	h.gameStateMutex.RUnlock()
	if err != nil {
		log.Printf("Error marshalling state: %v", err)
		return
	}

	msg, err := json.Marshal(Message{Type: "game_state", Payload: json.RawMessage(payload)})
	if err != nil {
		log.Printf("Error marshalling message: %v", err)
		return
	}

	for _, client := range h.clients {
		select {
		case client.send <- msg:
		default:
			log.Printf("Client %s send channel full. Skipping.", client.ID)
		}
	}
}

// --- Xử lý kết nối của Client ---
var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	for {
		_, msgBytes, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}
		if msg.Type == "player_ready" {
			var payload struct{ IsReady bool `json:"isReady"` }
			if json.Unmarshal(msg.Payload, &payload) == nil {
				c.hub.handlePlayerReady(c.ID, payload.IsReady)
			}
		}
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{
		ID:   conn.RemoteAddr().String(),
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
	}
	hub.register <- client

	idPayload, _ := json.Marshal(map[string]string{"id": client.ID})
	msg, _ := json.Marshal(Message{Type: "your_id", Payload: idPayload})
	client.send <- msg

	go client.writePump()
	go client.readPump()
}

// --- Hàm Main ---
func main() {
	rand.Seed(time.Now().UnixNano())
	hub := newHub()
	go hub.run()
	http.Handle("/", http.FileServer(http.Dir("../frontend")))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) { serveWs(hub, w, r) })
	log.Println("Server is running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("could not start server: %v\n", err)
	}
}
