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

// --- Constants ---
const (
	StartingChips = 1000
	SmallBlindAmt = 10
	BigBlindAmt   = 20
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
	playerReady    map[string]bool // Track ready status separately
	gameState      GameState
	gameStateMutex sync.RWMutex
}

// --- Structs cho Game ---
type Card struct {
	Suit string `json:"suit"`
	Rank string `json:"rank"`
}

type Player struct {
	ID          string `json:"id"`
	IsConnected bool   `json:"isConnected"`
	Hand        []Card `json:"hand"`
	Chips       int    `json:"chips"`
	Bet         int    `json:"bet"`
	IsInHand    bool   `json:"isInHand"`
}

type GameState struct {
	Players          map[string]Player `json:"players"`
	PlayerReady      map[string]bool   `json:"playerReady"`
	GameStarted      bool              `json:"gameStarted"`
	Deck             []Card            `json:"-"`
	Pot              int               `json:"pot"`
	PlayerOrder      []string          `json:"playerOrder"`
	DealerIndex      int               `json:"dealerIndex"`
	CurrentTurnIndex int               `json:"currentTurnIndex"`
	GamePhase        string            `json:"gamePhase"`
	LastBet          int               `json:"lastBet"`
	CommunityCards   []Card            `json:"communityCards"`
}

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// --- Khởi tạo Hub ---
func newHub() *Hub {
	return &Hub{
		clients:     make(map[string]*Client),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		playerReady: make(map[string]bool),
		gameState: GameState{
			Players:        make(map[string]Player),
			PlayerReady:    make(map[string]bool),
			DealerIndex:    -1,
			GamePhase:      "waiting",
			CommunityCards: []Card{},
		},
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client.ID] = client
			h.addOrUpdatePlayer(client.ID, true) // Mark as connected
			log.Printf("Client %s registered. Total clients: %d", client.ID, len(h.clients))

		case client := <-h.unregister:
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(client.send)
				h.addOrUpdatePlayer(client.ID, false) // Mark as disconnected
				log.Printf("Client %s unregistered. Total clients: %d", client.ID, len(h.clients))
			}
		}
	}
}

// --- Logic Game ---

func (h *Hub) addOrUpdatePlayer(playerID string, isConnected bool) {
	h.gameStateMutex.Lock()
	defer h.gameStateMutex.Unlock()

	player, exists := h.gameState.Players[playerID]
	if !exists {
		player = Player{ID: playerID, Hand: []Card{}, Chips: StartingChips}
		h.playerReady[playerID] = false
		log.Printf("Creating new player: %s", playerID)
	}
	player.IsConnected = isConnected
	h.gameState.Players[playerID] = player

	if !isConnected && h.gameState.GameStarted {
		player.IsInHand = false
		h.gameState.Players[playerID] = player
		log.Printf("Player %s disconnected mid-game, folding.", playerID)
		h.advanceTurnUnsafe()
	} else {
		h.broadcastGameStateUnsafe()
	}
}

func (h *Hub) handlePlayerReady(playerID string, isReady bool) {
	h.gameStateMutex.Lock()
	defer h.gameStateMutex.Unlock()

	if _, ok := h.playerReady[playerID]; ok {
		h.playerReady[playerID] = isReady
		log.Printf("Player %s set IsReady to: %t", playerID, isReady)
	}

	if h.gameState.GameStarted {
		h.broadcastGameStateUnsafe()
		return
	}

	eligiblePlayers := make(map[string]Player)
	for id, p := range h.gameState.Players {
		if p.IsConnected && p.Chips > 0 {
			eligiblePlayers[id] = p
		}
	}

	if len(eligiblePlayers) < 2 {
		log.Printf("Not enough eligible players to start. Have %d, need at least 2.", len(eligiblePlayers))
		h.broadcastGameStateUnsafe()
		return
	}

	allEligiblePlayersReady := true
	for id := range eligiblePlayers {
		if !h.playerReady[id] {
			allEligiblePlayersReady = false
			break
		}
	}

	if allEligiblePlayersReady {
		log.Printf("All %d eligible players are ready. Starting game.", len(eligiblePlayers))
		h.startGameUnsafe(eligiblePlayers)
	} else {
		log.Println("Waiting for all eligible players to be ready.")
	}

	h.broadcastGameStateUnsafe()
}

func (h *Hub) startGameUnsafe(activePlayers map[string]Player) {
	log.Println("--- GAME STARTING ---")
	h.gameState.GameStarted = true
	h.gameState.GamePhase = "pre-flop"
	h.gameState.Pot = 0
	h.gameState.LastBet = 0
	h.gameState.CommunityCards = []Card{}

	h.gameState.PlayerOrder = make([]string, 0, len(activePlayers))
	for id := range activePlayers {
		p := h.gameState.Players[id]
		p.Hand, p.Bet, p.IsInHand = []Card{}, 0, true
		h.gameState.Players[id] = p
		h.gameState.PlayerOrder = append(h.gameState.PlayerOrder, id)
	}

	for id := range h.playerReady {
		h.playerReady[id] = false
	}

	h.gameState.DealerIndex = (h.gameState.DealerIndex + 1) % len(h.gameState.PlayerOrder)

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

	for _, id := range h.gameState.PlayerOrder {
		p := h.gameState.Players[id]
		if len(h.gameState.Deck) > 1 {
			p.Hand = append(p.Hand, h.gameState.Deck[0], h.gameState.Deck[1])
			h.gameState.Deck = h.gameState.Deck[2:]
			h.gameState.Players[id] = p
		}
	}
	log.Printf("Dealt cards. Dealer is %s", h.gameState.PlayerOrder[h.gameState.DealerIndex])

	numPlayers := len(h.gameState.PlayerOrder)
	if numPlayers < 2 {
		h.endGameUnsafe("Not enough players to continue after dealing")
		return
	}
	sbIndex := (h.gameState.DealerIndex + 1) % numPlayers
	bbIndex := (h.gameState.DealerIndex + 2) % numPlayers

	h.handleBetUnsafe(h.gameState.PlayerOrder[sbIndex], SmallBlindAmt)
	log.Printf("Player %s is Small Blind, posts %d", h.gameState.PlayerOrder[sbIndex], SmallBlindAmt)
	h.handleBetUnsafe(h.gameState.PlayerOrder[bbIndex], BigBlindAmt)
	log.Printf("Player %s is Big Blind, posts %d", h.gameState.PlayerOrder[bbIndex], BigBlindAmt)

	h.gameState.LastBet = BigBlindAmt
	h.gameState.CurrentTurnIndex = (bbIndex + 1) % numPlayers
	log.Printf("First turn is %s", h.gameState.PlayerOrder[h.gameState.CurrentTurnIndex])
}

func (h *Hub) endGameUnsafe(reason string) {
	log.Printf("--- GAME ENDED: %s ---", reason)
	h.gameState.GameStarted = false
	h.gameState.GamePhase = "waiting"

	for id, p := range h.gameState.Players {
		p.Hand = []Card{}
		p.Bet = 0
		p.IsInHand = false
		h.gameState.Players[id] = p
	}
	h.gameState.CommunityCards = []Card{}
	h.gameState.Pot = 0
}

// broadcastGameStateUnsafe sends the current game state to all connected clients.
// It assumes the caller is holding a lock on gameStateMutex.
func (h *Hub) broadcastGameStateUnsafe() {
	h.gameState.PlayerReady = h.playerReady

	payload, err := json.Marshal(h.gameState)
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

// --- Functions below this line are mostly unchanged ---

func (h *Hub) handleBetUnsafe(playerID string, amount int) {
	if player, ok := h.gameState.Players[playerID]; ok {
		actualAmount := amount
		if player.Chips < amount {
			actualAmount = player.Chips // All-in
		}
		player.Chips -= actualAmount
		player.Bet += actualAmount
		h.gameState.Players[playerID] = player
	}
}

type PlayerActionPayload struct {
	Action string `json:"action"`
	Amount int    `json:"amount"`
}

func (h *Hub) handlePlayerAction(playerID string, payloadBytes json.RawMessage) {
	h.gameStateMutex.Lock()
	defer h.gameStateMutex.Unlock()

	if !h.gameState.GameStarted || len(h.gameState.PlayerOrder) == 0 || h.gameState.PlayerOrder[h.gameState.CurrentTurnIndex] != playerID {
		return
	}

	var payload PlayerActionPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		log.Printf("Error unmarshalling action payload: %v", err)
		return
	}

	log.Printf("Player %s action: %s (Amount: %d)", playerID, payload.Action, payload.Amount)

	player := h.gameState.Players[playerID]

	switch payload.Action {
	case "fold":
		player.IsInHand = false
		h.gameState.Players[playerID] = player
	case "check":
		if player.Bet < h.gameState.LastBet {
			return
		}
	case "call":
		amountToCall := h.gameState.LastBet - player.Bet
		h.handleBetUnsafe(playerID, amountToCall)
	case "raise":
		totalBet := payload.Amount
		if totalBet < h.gameState.LastBet*2 {
			return
		}
		amountToBet := totalBet - player.Bet
		h.handleBetUnsafe(playerID, amountToBet)
		h.gameState.LastBet = totalBet
	}

	h.advanceTurnUnsafe()
}

func (h *Hub) advanceTurnUnsafe() {
	playersInHandCount := 0
	var lastPlayerInHandID string
	for _, id := range h.gameState.PlayerOrder {
		p := h.gameState.Players[id]
		if p.IsInHand && p.IsConnected {
			playersInHandCount++
			lastPlayerInHandID = id
		}
	}
	if playersInHandCount <= 1 {
		h.awardPotUnsafe(lastPlayerInHandID)
		h.endGameUnsafe(lastPlayerInHandID + " wins by default!")
		h.broadcastGameStateUnsafe()
		return
	}

	roundOver := true
	playersAllInCount := 0
	activePlayerCount := 0
	for _, id := range h.gameState.PlayerOrder {
		p := h.gameState.Players[id]
		if p.IsInHand && p.IsConnected {
			activePlayerCount++
			if p.Chips == 0 {
				playersAllInCount++
			} else if p.Bet < h.gameState.LastBet {
				roundOver = false
			}
		}
	}
	if activePlayerCount > 0 && activePlayerCount == playersAllInCount {
		roundOver = true
	}

	if roundOver {
		log.Printf("--- BETTING ROUND ENDED (%s) ---", h.gameState.GamePhase)
		h.startNextPhaseUnsafe()
		return
	}

	startTurnIndex := h.gameState.CurrentTurnIndex
	for i := 0; i < len(h.gameState.PlayerOrder); i++ {
		h.gameState.CurrentTurnIndex = (startTurnIndex + 1 + i) % len(h.gameState.PlayerOrder)
		nextPlayerID := h.gameState.PlayerOrder[h.gameState.CurrentTurnIndex]
		if player, ok := h.gameState.Players[nextPlayerID]; ok {
			if player.IsInHand && player.Chips > 0 && player.IsConnected {
				log.Printf("Next turn is %s", h.gameState.PlayerOrder[h.gameState.CurrentTurnIndex])
				h.broadcastGameStateUnsafe()
				return
			}
		}
	}
}

func (h *Hub) startNextPhaseUnsafe() {
	h.gameState.Pot += h.collectBetsUnsafe()

	playersInHandCount := 0
	var lastPlayerInHandID string
	for _, id := range h.gameState.PlayerOrder {
		p := h.gameState.Players[id]
		if p.IsInHand && p.IsConnected {
			playersInHandCount++
			lastPlayerInHandID = id
		}
	}
	if playersInHandCount <= 1 {
		h.awardPotUnsafe(lastPlayerInHandID)
		h.endGameUnsafe(lastPlayerInHandID + " wins!")
		return
	}

	switch h.gameState.GamePhase {
	case "pre-flop":
		h.gameState.GamePhase = "flop"
		h.dealCommunityCardsUnsafe(3)
	case "flop":
		h.gameState.GamePhase = "turn"
		h.dealCommunityCardsUnsafe(1)
	case "turn":
		h.gameState.GamePhase = "river"
		h.dealCommunityCardsUnsafe(1)
	case "river":
		h.gameState.GamePhase = "showdown"
		h.endGameUnsafe("Showdown!")
		return
	}

	h.gameState.LastBet = 0
	h.gameState.CurrentTurnIndex = h.gameState.DealerIndex
	for i := 0; i < len(h.gameState.PlayerOrder); i++ {
		h.gameState.CurrentTurnIndex = (h.gameState.CurrentTurnIndex + 1) % len(h.gameState.PlayerOrder)
		playerID := h.gameState.PlayerOrder[h.gameState.CurrentTurnIndex]
		p := h.gameState.Players[playerID]
		if p.IsInHand && p.IsConnected {
			break
		}
	}
}

func (h *Hub) dealCommunityCardsUnsafe(count int) {
	if len(h.gameState.Deck) > 1 {
		h.gameState.Deck = h.gameState.Deck[1:]
	}
	if len(h.gameState.Deck) >= count {
		h.gameState.CommunityCards = append(h.gameState.CommunityCards, h.gameState.Deck[:count]...)
		h.gameState.Deck = h.gameState.Deck[count:]
	}
}

func (h *Hub) collectBetsUnsafe() int {
	collected := 0
	for id, p := range h.gameState.Players {
		collected += p.Bet
		p.Bet = 0
		h.gameState.Players[id] = p
	}
	return collected
}

func (h *Hub) awardPotUnsafe(winnerID string) {
	if winnerID != "" {
		h.gameState.Pot += h.collectBetsUnsafe()
		if winner, ok := h.gameState.Players[winnerID]; ok {
			winner.Chips += h.gameState.Pot
			h.gameState.Players[winnerID] = winner
			log.Printf("Awarded pot of %d to %s", h.gameState.Pot, winnerID)
			h.gameState.Pot = 0
		}
	}
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	for {
		_, msgBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			log.Printf("Error unmarshalling message: %v", err)
			continue
		}

		switch msg.Type {
		case "player_ready":
			var payload struct{ IsReady bool `json:"isReady"` }
			if json.Unmarshal(msg.Payload, &payload) == nil {
				c.hub.handlePlayerReady(c.ID, payload.IsReady)
			}
		case "player_action":
			c.hub.handlePlayerAction(c.ID, msg.Payload)
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
