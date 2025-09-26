package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid" // <-- THÊM DÒNG NÀY
	"github.com/gorilla/websocket"
)

// --- Constants ---
const (
	StartingChips = 1000
	SmallBlindAmt = 10
	BigBlindAmt   = 20
)

// Global deck pool for reusing card objects
var (
	deckPool = sync.Pool{
		New: func() interface{} {
			suits := []string{"♥", "♦", "♣", "♠"}
			ranks := []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A"}
			deck := make([]Card, 0, 52)
			for _, s := range suits {
				for _, r := range ranks {
					deck = append(deck, Card{Suit: s, Rank: r})
				}
			}
			return deck
		},
	}
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
	playerReady    map[string]bool
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
	Name        string `json:"name"`
	IsConnected bool   `json:"isConnected"`
	Hand        []Card `json:"hand"`
	Chips       int    `json:"chips"`
	Bet         int    `json:"bet"`
	IsInHand    bool   `json:"isInHand"`
	IsAllIn     bool   `json:"isAllIn"`
	HasActed    bool   `json:"hasActed"`
}

type GameState struct {
	Players          map[string]Player `json:"players"`
	PlayerReady      map[string]bool   `json:"playerReady"`
	GameStarted      bool              `json:"gameStarted"`
	Deck             []Card            `json:"-"`
	Pot              int               `json:"pot"`
	SidePots         []SidePot         `json:"sidePots"`
	PlayerOrder      []string          `json:"playerOrder"`
	DealerIndex      int               `json:"dealerIndex"`
	CurrentTurnIndex int               `json:"currentTurnIndex"`
	GamePhase        string            `json:"gamePhase"`
	LastBet          int               `json:"lastBet"`
	MinRaise         int               `json:"minRaise"`
	CommunityCards   []Card            `json:"communityCards"`
	WinningHandDesc  string            `json:"winningHandDesc,omitempty"`
	ChatMessages     []ChatMessage     `json:"chatMessages"`
	actionToPlayerID string
}

type SidePot struct {
	Amount      int      `json:"amount"`
	EligibleIDs []string `json:"eligibleIds"`
}

type ChatMessage struct {
	PlayerID  string    `json:"playerId"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

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
			SidePots:       []SidePot{},
			ChatMessages:   []ChatMessage{},
			MinRaise:       BigBlindAmt,
		},
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client.ID] = client
			h.addOrUpdatePlayer(client.ID, true)
		case client := <-h.unregister:
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(client.send)
				h.addOrUpdatePlayer(client.ID, false)
			}
		}
	}
}

func (h *Hub) addOrUpdatePlayer(playerID string, isConnected bool) {
	h.addOrUpdatePlayerWithName(playerID, isConnected, "")
}

func (h *Hub) addOrUpdatePlayerWithName(playerID string, isConnected bool, name string) {
	h.gameStateMutex.Lock()
	defer h.gameStateMutex.Unlock()
	player, exists := h.gameState.Players[playerID]
	if !exists {
		playerName := name
		if playerName == "" {
			playerName = "Player-" + playerID[:8] // Default name
		}
		player = Player{
			ID:          playerID, 
			Name:        playerName,
			Hand:        []Card{}, 
			Chips:       StartingChips,
			IsAllIn:     false,
			HasActed:    false,
		}
		h.playerReady[playerID] = false
	}
	if name != "" && name != player.Name {
		player.Name = name
	}
	player.IsConnected = isConnected
	h.gameState.Players[playerID] = player
	if !isConnected && h.gameState.GameStarted && player.IsInHand {
		player.IsInHand = false
		h.gameState.Players[playerID] = player
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
		h.startGameUnsafe(eligiblePlayers)
	}
	h.broadcastGameStateUnsafe()
}

func (h *Hub) handleChatMessage(playerID string, payloadBytes json.RawMessage) {
	var payload ChatPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		log.Printf("Error unmarshaling chat payload: %v", err)
		return
	}
	
	h.gameStateMutex.Lock()
	defer h.gameStateMutex.Unlock()
	
	if player, exists := h.gameState.Players[playerID]; exists {
		chatMessage := ChatMessage{
			PlayerID:  playerID,
			Message:   payload.Message,
			Timestamp: time.Now(),
		}
		h.gameState.ChatMessages = append(h.gameState.ChatMessages, chatMessage)
		
		// Keep only last 50 messages
		if len(h.gameState.ChatMessages) > 50 {
			h.gameState.ChatMessages = h.gameState.ChatMessages[len(h.gameState.ChatMessages)-50:]
		}
		
		log.Printf("Chat from %s: %s", player.Name, payload.Message)
		h.broadcastGameStateUnsafe()
	}
}

func (h *Hub) handlePlayerJoin(playerID string, payloadBytes json.RawMessage) {
	var payload PlayerJoinPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		log.Printf("Error unmarshaling player join payload: %v", err)
		return
	}
	
	h.addOrUpdatePlayerWithName(playerID, true, payload.Name)
}

func (h *Hub) startGameUnsafe(activePlayers map[string]Player) {
	log.Println("--- GAME STARTING ---")
	h.gameState.GameStarted = true
	h.gameState.GamePhase = "pre-flop"
	h.gameState.Pot = 0
	h.gameState.CommunityCards = []Card{}
	h.gameState.WinningHandDesc = ""
	h.gameState.PlayerOrder = make([]string, 0, len(activePlayers))
	for id := range activePlayers {
		p := h.gameState.Players[id]
		p.Hand, p.Bet, p.IsInHand = []Card{}, 0, true
		p.IsAllIn = false
		p.HasActed = false
		h.gameState.Players[id] = p
		h.gameState.PlayerOrder = append(h.gameState.PlayerOrder, id)
	}
	for id := range h.playerReady {
		h.playerReady[id] = false
	}
	
	h.addSystemChatMessage(fmt.Sprintf("Game started with %d players!", len(activePlayers)))
	
	h.gameState.DealerIndex = (h.gameState.DealerIndex + 1) % len(h.gameState.PlayerOrder)
	
	// Use deck pool for better memory management
	deck := deckPool.Get().([]Card)
	// Create a copy to avoid modifying the pooled deck
	gameDeck := make([]Card, len(deck))
	copy(gameDeck, deck)
	deckPool.Put(deck) // Return to pool immediately
	
	rand.Shuffle(len(gameDeck), func(i, j int) { gameDeck[i], gameDeck[j] = gameDeck[j], gameDeck[i] })
	h.gameState.Deck = gameDeck
	for _, id := range h.gameState.PlayerOrder {
		if len(h.gameState.Deck) > 1 {
			p := h.gameState.Players[id]
			p.Hand = append(p.Hand, h.gameState.Deck[0], h.gameState.Deck[1])
			h.gameState.Deck = h.gameState.Deck[2:]
			h.gameState.Players[id] = p
		}
	}
	numPlayers := len(h.gameState.PlayerOrder)
	sbIndex := (h.gameState.DealerIndex + 1) % numPlayers
	bbIndex := (h.gameState.DealerIndex + 2) % numPlayers

	h.handleBetUnsafe(h.gameState.PlayerOrder[sbIndex], SmallBlindAmt)
	h.handleBetUnsafe(h.gameState.PlayerOrder[bbIndex], BigBlindAmt)

	h.gameState.LastBet = BigBlindAmt
	h.gameState.CurrentTurnIndex = (bbIndex + 1) % len(h.gameState.PlayerOrder)
	h.gameState.actionToPlayerID = h.gameState.PlayerOrder[bbIndex]
}

func (h *Hub) endGameUnsafe(reason string) {
	log.Printf("--- GAME ENDED: %s ---", reason)
	h.gameState.GameStarted = false
	h.gameState.GamePhase = "waiting"
	
	// Check for player elimination and reset game state
	eliminatedPlayers := []string{}
	for id := range h.gameState.Players {
		p := h.gameState.Players[id]
		p.Hand, p.Bet, p.IsInHand = []Card{}, 0, false
		p.IsAllIn = false
		p.HasActed = false
		
		// Eliminate players with 0 chips
		if p.Chips <= 0 {
			eliminatedPlayers = append(eliminatedPlayers, id)
			log.Printf("Player %s eliminated (no chips remaining)", p.Name)
		}
		
		h.gameState.Players[id] = p
	}
	
	// Remove eliminated players
	for _, id := range eliminatedPlayers {
		delete(h.gameState.Players, id)
		delete(h.playerReady, id)
	}
	
	h.gameState.CommunityCards = []Card{}
	h.gameState.SidePots = []SidePot{}
	h.gameState.Pot = 0
	h.gameState.MinRaise = BigBlindAmt
	
	if len(eliminatedPlayers) > 0 {
		h.addSystemChatMessage(fmt.Sprintf("%d player(s) eliminated", len(eliminatedPlayers)))
	}
}

func (h *Hub) addSystemChatMessage(message string) {
	chatMessage := ChatMessage{
		PlayerID:  "system",
		Message:   message,
		Timestamp: time.Now(),
	}
	h.gameState.ChatMessages = append(h.gameState.ChatMessages, chatMessage)
	
	// Keep only last 50 messages
	if len(h.gameState.ChatMessages) > 50 {
		h.gameState.ChatMessages = h.gameState.ChatMessages[len(h.gameState.ChatMessages)-50:]
	}
}

func (h *Hub) broadcastGameStateUnsafe() {
	h.gameState.PlayerReady = h.playerReady
	payload, err := json.Marshal(h.gameState)
	if err != nil {
		log.Printf("Error marshaling game state: %v", err)
		return
	}
	msg, err := json.Marshal(Message{Type: "game_state", Payload: json.RawMessage(payload)})
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}
	// Pre-marshal once and reuse for all clients
	for _, client := range h.clients {
		select {
		case client.send <- msg:
		default:
			// Client channel is full, close connection
			log.Printf("Client %s channel full, closing connection", client.ID)
			close(client.send)
			delete(h.clients, client.ID)
		}
	}
}

func (h *Hub) handleBetUnsafe(playerID string, amount int) {
	if player, ok := h.gameState.Players[playerID]; ok {
		actualAmount := amount
		if player.Chips < amount {
			actualAmount = player.Chips
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

type ChatPayload struct {
	Message string `json:"message"`
}

type PlayerJoinPayload struct {
	Name string `json:"name"`
}

func (h *Hub) handlePlayerAction(playerID string, payloadBytes json.RawMessage) {
	h.gameStateMutex.Lock()
	defer h.gameStateMutex.Unlock()

	if h.gameState.GamePhase == "showdown" {
		return
	}

	if !h.gameState.GameStarted || len(h.gameState.PlayerOrder) == 0 || h.gameState.CurrentTurnIndex < 0 || h.gameState.PlayerOrder[h.gameState.CurrentTurnIndex] != playerID {
		return
	}
	var payload PlayerActionPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return
	}

	player := h.gameState.Players[playerID]
	if player.IsAllIn || !player.IsInHand {
		return // Player can't act if all-in or folded
	}
	
	player.HasActed = true
	roundIsOver := false

	switch payload.Action {
	case "fold":
		player.IsInHand = false
		log.Printf("Player %s folded", player.Name)
		
	case "check":
		if player.Bet < h.gameState.LastBet {
			return // Can't check if there's a bet to call
		}
		log.Printf("Player %s checked", player.Name)
		if playerID == h.gameState.actionToPlayerID {
			roundIsOver = true
		}
		
	case "call":
		amountToCall := h.gameState.LastBet - player.Bet
		if amountToCall <= 0 {
			// No amount to call, treat as check
			if playerID == h.gameState.actionToPlayerID {
				roundIsOver = true
			}
		} else {
			actualCall := amountToCall
			if player.Chips < amountToCall {
				actualCall = player.Chips // All-in call
				player.IsAllIn = true
			}
			h.handleBetUnsafe(playerID, actualCall)
			log.Printf("Player %s called %d (all-in: %v)", player.Name, actualCall, player.IsAllIn)
			if playerID == h.gameState.actionToPlayerID {
				roundIsOver = true
			}
		}
		
	case "raise":
		totalBet := payload.Amount
		amountToBet := totalBet - player.Bet
		
		// Validate minimum raise
		minRaise := h.gameState.LastBet + h.gameState.MinRaise
		if totalBet < minRaise && player.Chips > amountToBet {
			return // Invalid raise amount
		}
		
		if player.Chips <= amountToBet {
			// All-in raise
			actualBet := player.Chips
			h.handleBetUnsafe(playerID, actualBet)
			player.IsAllIn = true
			log.Printf("Player %s raised all-in with %d", player.Name, player.Bet)
		} else {
			h.handleBetUnsafe(playerID, amountToBet)
			h.gameState.LastBet = totalBet
			h.gameState.MinRaise = amountToBet // The raise amount, not the difference
			log.Printf("Player %s raised to %d", player.Name, totalBet)
		}
		h.gameState.actionToPlayerID = playerID
		
		// Reset HasActed for all players except this one
		for id, p := range h.gameState.Players {
			if id != playerID && p.IsInHand && !p.IsAllIn {
				p.HasActed = false
				h.gameState.Players[id] = p
			}
		}
	}

	h.gameState.Players[playerID] = player

	// Check if round should end
	if roundIsOver || h.shouldEndRound() {
		h.startNextPhaseUnsafe()
	} else {
		h.advanceTurnUnsafe()
	}
}

// Check if the betting round should end
func (h *Hub) shouldEndRound() bool {
	playersInHand := 0
	playersWhoCanAct := 0
	playersWhoActed := 0
	
	for _, p := range h.gameState.Players {
		if p.IsInHand {
			playersInHand++
			if !p.IsAllIn {
				playersWhoCanAct++
				// In new betting rounds with no bets, check if player has acted
				// In rounds with bets, check if player has acted and matched the bet
				if h.gameState.LastBet == 0 {
					if p.HasActed {
						playersWhoActed++
					}
				} else {
					if p.HasActed && p.Bet == h.gameState.LastBet {
						playersWhoActed++
					}
				}
			}
		}
	}
	
	// Round ends if only one player left or all who can act have acted and called/checked
	return playersInHand <= 1 || (playersWhoCanAct > 0 && playersWhoActed == playersWhoCanAct)
}

func (h *Hub) advanceTurnUnsafe() {
	playersInHandCount := 0
	var lastPlayerInHandID string
	for _, id := range h.gameState.PlayerOrder {
		if p := h.gameState.Players[id]; p.IsInHand && p.IsConnected {
			playersInHandCount++
			lastPlayerInHandID = id
		}
	}
	if playersInHandCount <= 1 {
		if lastPlayerInHandID != "" {
			h.awardPotUnsafe([]string{lastPlayerInHandID})
			h.endGameUnsafe(lastPlayerInHandID + " wins by default!")
		} else {
			h.endGameUnsafe("No players left.")
		}
		h.broadcastGameStateUnsafe()
		return
	}

	startTurnIndex := h.gameState.CurrentTurnIndex
	log.Printf("Advancing turn from index %d", startTurnIndex)
	
	for i := 1; i <= len(h.gameState.PlayerOrder); i++ {
		h.gameState.CurrentTurnIndex = (startTurnIndex + i) % len(h.gameState.PlayerOrder)
		nextPlayerID := h.gameState.PlayerOrder[h.gameState.CurrentTurnIndex]
		if player, ok := h.gameState.Players[nextPlayerID]; ok {
			log.Printf("Checking player %s: InHand=%v, AllIn=%v, Connected=%v", 
				player.Name, player.IsInHand, player.IsAllIn, player.IsConnected)
			// Skip all-in players and players not in hand
			if player.IsInHand && !player.IsAllIn && player.IsConnected {
				log.Printf("Turn advanced to player %s (index %d)", player.Name, h.gameState.CurrentTurnIndex)
				h.broadcastGameStateUnsafe()
				return
			}
		}
	}

	// All remaining players are all-in or can't act, go to next phase
	log.Printf("No valid players found for next turn, advancing to next phase")
	h.startNextPhaseUnsafe()
}

func (h *Hub) startNextPhaseUnsafe() {
	log.Printf("--- Advancing phase from %s ---", h.gameState.GamePhase)
	h.gameState.Pot += h.collectBetsUnsafe()

	if h.gameState.GamePhase == "river" {
		h.gameState.GamePhase = "showdown"
		h.handleShowdownUnsafe()
		return
	}

	// Reset HasActed for all players for the new betting round
	for id, p := range h.gameState.Players {
		if p.IsInHand && !p.IsAllIn {
			p.HasActed = false
			h.gameState.Players[id] = p
			log.Printf("Reset HasActed for player %s", p.Name)
		}
	}

	// Find the first player to act (after dealer, but skip all-in and folded players)
	firstPlayerIndex := -1
	if len(h.gameState.PlayerOrder) > 0 {
		for i := 1; i <= len(h.gameState.PlayerOrder); i++ {
			idx := (h.gameState.DealerIndex + i) % len(h.gameState.PlayerOrder)
			playerID := h.gameState.PlayerOrder[idx]
			if p, ok := h.gameState.Players[playerID]; ok && p.IsInHand && p.IsConnected && !p.IsAllIn {
				firstPlayerIndex = idx
				break
			}
		}
	}

	h.gameState.CurrentTurnIndex = firstPlayerIndex
	h.gameState.actionToPlayerID = "" // Reset action tracking

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
	}

	h.gameState.LastBet = 0       // Reset betting for new round
	h.gameState.MinRaise = BigBlindAmt // Reset minimum raise

	if h.gameState.CurrentTurnIndex == -1 {
		// No players can act (all all-in), go to next phase
		h.broadcastGameStateUnsafe()
		time.AfterFunc(1*time.Second, func() {
			h.gameStateMutex.Lock()
			defer h.gameStateMutex.Unlock()
			h.startNextPhaseUnsafe()
		})
	} else {
		h.broadcastGameStateUnsafe()
	}
}

func (h *Hub) dealCommunityCardsUnsafe(count int) {
	if len(h.gameState.Deck) > 0 {
		h.gameState.Deck = h.gameState.Deck[1:]
	}
	if len(h.gameState.Deck) >= count {
		h.gameState.CommunityCards = append(h.gameState.CommunityCards, h.gameState.Deck[:count]...)
		h.gameState.Deck = h.gameState.Deck[count:]
	}
}

func (h *Hub) collectBetsUnsafe() int {
	collected := 0
	for id := range h.gameState.Players {
		p := h.gameState.Players[id]
		collected += p.Bet
		p.Bet = 0
		h.gameState.Players[id] = p
	}
	return collected
}

func (h *Hub) awardPotUnsafe(winnerIDs []string) {
	if len(winnerIDs) == 0 {
		return
	}

	h.gameState.Pot += h.collectBetsUnsafe()
	share := h.gameState.Pot / len(winnerIDs)
	remainder := h.gameState.Pot % len(winnerIDs)

	for _, winnerID := range winnerIDs {
		if winner, ok := h.gameState.Players[winnerID]; ok {
			winner.Chips += share
			h.gameState.Players[winnerID] = winner
		}
	}

	if remainder > 0 {
		for i := 1; i <= len(h.gameState.PlayerOrder); i++ {
			playerID := h.gameState.PlayerOrder[(h.gameState.DealerIndex+i)%len(h.gameState.PlayerOrder)]
			if slices.Contains(winnerIDs, playerID) {
				if winner, ok := h.gameState.Players[playerID]; ok {
					winner.Chips++
					h.gameState.Players[playerID] = winner
					remainder--
					if remainder == 0 {
						break
					}
				}
			}
		}
	}
	h.gameState.Pot = 0
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	
	// Set read deadline and pong handler
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	
	for {
		_, msgBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for client %s: %v", c.ID, err)
			}
			break
		}
		
		// Reset read deadline on successful message
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		
		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			log.Printf("Invalid message from client %s: %v", c.ID, err)
			continue
		}
		switch msg.Type {
		case "player_ready":
			var payload struct {
				IsReady bool `json:"isReady"`
			}
			if json.Unmarshal(msg.Payload, &payload) == nil {
				c.hub.handlePlayerReady(c.ID, payload.IsReady)
			} else {
				log.Printf("Invalid player_ready payload from client %s", c.ID)
			}
		case "player_action":
			c.hub.handlePlayerAction(c.ID, msg.Payload)
		case "chat_message":
			c.hub.handleChatMessage(c.ID, msg.Payload)
		case "player_join":
			c.hub.handlePlayerJoin(c.ID, msg.Payload)
		default:
			log.Printf("Unknown message type '%s' from client %s", msg.Type, c.ID)
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second) // Ping every 54 seconds
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("Error writing message to client %s: %v", c.ID, err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Error sending ping to client %s: %v", c.ID, err)
				return
			}
		}
	}
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	// *** SỬA ĐỔI CHÍNH Ở ĐÂY ***
	// Thay vì dùng địa chỉ IP, chúng ta tạo một ID duy nhất
	client := &Client{ID: uuid.New().String(), hub: hub, conn: conn, send: make(chan []byte, 256)}
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
	
	// Start performance monitoring
	hub.startMetricsLogger()
	
	http.Handle("/", http.FileServer(http.Dir("../frontend")))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) { serveWs(hub, w, r) })
	log.Println("Server is running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("could not start server: %v\n", err)
	}
}

// --- SHOWDOWN LOGIC ---

type HandRank int

const (
	HighCard HandRank = iota
	OnePair
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
)

var handRankStrings = map[HandRank]string{
	HighCard:      "High Card",
	OnePair:       "One Pair",
	TwoPair:       "Two Pair",
	ThreeOfAKind:  "Three of a Kind",
	Straight:      "Straight",
	Flush:         "Flush",
	FullHouse:     "Full House",
	FourOfAKind:   "Four of a Kind",
	StraightFlush: "Straight Flush",
}

type EvaluatedHand struct {
	PlayerID string
	Rank     HandRank
	Values   []int
}

func rankToInt(rank string) int {
	switch rank {
	case "2":
		return 2
	case "3":
		return 3
	case "4":
		return 4
	case "5":
		return 5
	case "6":
		return 6
	case "7":
		return 7
	case "8":
		return 8
	case "9":
		return 9
	case "10":
		return 10
	case "J":
		return 11
	case "Q":
		return 12
	case "K":
		return 13
	case "A":
		return 14
	}
	return 0
}
func evaluateHand(cards []Card) EvaluatedHand {
	rankCounts := make(map[int]int)
	suitCounts := make(map[string][]int)
	for _, c := range cards {
		rankVal := rankToInt(c.Rank)
		rankCounts[rankVal]++
		suitCounts[c.Suit] = append(suitCounts[c.Suit], rankVal)
	}
	var flushSuit string
	for suit, ranks := range suitCounts {
		if len(ranks) >= 5 {
			flushSuit = suit
			break
		}
	}
	if flushSuit != "" {
		flushRanks := suitCounts[flushSuit]
		sort.Sort(sort.Reverse(sort.IntSlice(flushRanks)))
		straight, highCard := findStraight(flushRanks)
		if straight {
			return EvaluatedHand{Rank: StraightFlush, Values: []int{highCard}}
		}
		return EvaluatedHand{Rank: Flush, Values: flushRanks[:5]}
	}
	var quads, trips, pair1, pair2 int
	var kickers []int
	for rank, count := range rankCounts {
		if count == 4 {
			quads = rank
		} else if count == 3 {
			if rank > trips {
				trips = rank
			}
		} else if count == 2 {
			if rank > pair1 {
				pair2 = pair1
				pair1 = rank
			} else if rank > pair2 {
				pair2 = rank
			}
		} else {
			kickers = append(kickers, rank)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(kickers)))
	if quads > 0 {
		kicker := 0
		if trips > 0 {
			kicker = trips
		} else if pair1 > 0 {
			kicker = pair1
		} else if len(kickers) > 0 {
			kicker = kickers[0]
		}
		return EvaluatedHand{Rank: FourOfAKind, Values: []int{quads, kicker}}
	}
	if trips > 0 && pair1 > 0 {
		return EvaluatedHand{Rank: FullHouse, Values: []int{trips, pair1}}
	}
	var allRanks []int
	for r := range rankCounts {
		allRanks = append(allRanks, r)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(allRanks)))
	straight, highCard := findStraight(allRanks)
	if straight {
		return EvaluatedHand{Rank: Straight, Values: []int{highCard}}
	}
	if trips > 0 {
		values := append([]int{trips}, kickers...)
		return EvaluatedHand{Rank: ThreeOfAKind, Values: values[:3]}
	}
	if pair1 > 0 && pair2 > 0 {
		values := append([]int{pair1, pair2}, kickers...)
		return EvaluatedHand{Rank: TwoPair, Values: values[:3]}
	}
	if pair1 > 0 {
		values := append([]int{pair1}, kickers...)
		return EvaluatedHand{Rank: OnePair, Values: values[:4]}
	}
	return EvaluatedHand{Rank: HighCard, Values: kickers[:5]}
}
func findStraight(uniqueSortedRanks []int) (bool, int) {
	if slices.Contains(uniqueSortedRanks, 14) && slices.Contains(uniqueSortedRanks, 2) && slices.Contains(uniqueSortedRanks, 3) && slices.Contains(uniqueSortedRanks, 4) && slices.Contains(uniqueSortedRanks, 5) {
		return true, 5
	}
	for i := 0; i <= len(uniqueSortedRanks)-5; i++ {
		isStraight := true
		for j := 0; j < 4; j++ {
			if uniqueSortedRanks[i+j] != uniqueSortedRanks[i+j+1]+1 {
				isStraight = false
				break
			}
		}
		if isStraight {
			return true, uniqueSortedRanks[i]
		}
	}
	return false, 0
}
func compareHands(h1, h2 EvaluatedHand) int {
	if h1.Rank > h2.Rank {
		return 1
	}
	if h1.Rank < h2.Rank {
		return -1
	}
	for i := 0; i < len(h1.Values); i++ {
		if h1.Values[i] > h2.Values[i] {
			return 1
		}
		if h1.Values[i] < h2.Values[i] {
			return -1
		}
	}
	return 0
}
func (h *Hub) handleShowdownUnsafe() {
	h.gameState.CurrentTurnIndex = -1 // Explicitly end turn-based action

	var winners []string
	var bestHand EvaluatedHand
	firstPlayer := true
	for _, id := range h.gameState.PlayerOrder {
		if p, ok := h.gameState.Players[id]; ok && p.IsInHand && p.IsConnected {
			currentHand := evaluateHand(append(p.Hand, h.gameState.CommunityCards...))
			currentHand.PlayerID = id
			if firstPlayer {
				bestHand = currentHand
				winners = []string{id}
				firstPlayer = false
			} else {
				switch compareHands(currentHand, bestHand) {
				case 1:
					bestHand = currentHand
					winners = []string{id}
				case 0:
					winners = append(winners, id)
				}
			}
		}
	}
	if len(winners) > 0 {
		if len(winners) == 1 {
			winnerID := winners[0]
			h.gameState.WinningHandDesc = "Winner is " + winnerID[:5] + " with " + handRankStrings[bestHand.Rank]
		} else {
			h.gameState.WinningHandDesc = "Split pot!"
		}
		h.awardPotUnsafe(winners)
	}
	h.broadcastGameStateUnsafe()
	time.AfterFunc(5*time.Second, func() {
		h.gameStateMutex.Lock()
		defer h.gameStateMutex.Unlock()
		h.endGameUnsafe("Showdown finished.")
		h.broadcastGameStateUnsafe()
	})
}
