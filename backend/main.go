package main

import (
	"encoding/json"
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
	WinningHandDesc  string            `json:"winningHandDesc,omitempty"`
	actionToPlayerID string
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
	h.gameStateMutex.Lock()
	defer h.gameStateMutex.Unlock()
	player, exists := h.gameState.Players[playerID]
	if !exists {
		player = Player{ID: playerID, Hand: []Card{}, Chips: StartingChips}
		h.playerReady[playerID] = false
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
	for id := range h.gameState.Players {
		p := h.gameState.Players[id]
		p.Hand, p.Bet, p.IsInHand = []Card{}, 0, false
		h.gameState.Players[id] = p
	}
	h.gameState.CommunityCards = []Card{}
	h.gameState.Pot = 0
}

func (h *Hub) broadcastGameStateUnsafe() {
	h.gameState.PlayerReady = h.playerReady
	payload, err := json.Marshal(h.gameState)
	if err != nil {
		return
	}
	msg, err := json.Marshal(Message{Type: "game_state", Payload: json.RawMessage(payload)})
	if err != nil {
		return
	}
	for _, client := range h.clients {
		select {
		case client.send <- msg:
		default:
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
	roundIsOver := false

	switch payload.Action {
	case "fold":
		player.IsInHand = false
	case "check":
		if player.Bet < h.gameState.LastBet {
			return
		}
		if playerID == h.gameState.actionToPlayerID {
			roundIsOver = true
		}
	case "call":
		amountToCall := h.gameState.LastBet - player.Bet
		h.handleBetUnsafe(playerID, amountToCall)
		if playerID == h.gameState.actionToPlayerID {
			roundIsOver = true
		}
	case "raise":
		totalBet := payload.Amount
		amountToBet := totalBet - player.Bet
		if totalBet < h.gameState.LastBet*2 && player.Chips > amountToBet {
			return
		}
		h.handleBetUnsafe(playerID, amountToBet)
		h.gameState.LastBet = totalBet
		h.gameState.actionToPlayerID = playerID
	}

	h.gameState.Players[playerID] = player

	if roundIsOver {
		h.startNextPhaseUnsafe()
	} else {
		h.advanceTurnUnsafe()
	}
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
	for i := 1; i <= len(h.gameState.PlayerOrder); i++ {
		h.gameState.CurrentTurnIndex = (startTurnIndex + i) % len(h.gameState.PlayerOrder)
		nextPlayerID := h.gameState.PlayerOrder[h.gameState.CurrentTurnIndex]
		if player, ok := h.gameState.Players[nextPlayerID]; ok {
			if player.IsInHand && player.Chips > 0 && player.IsConnected {
				h.broadcastGameStateUnsafe()
				return
			}
		}
	}

	// All remaining players are all-in
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

	firstPlayerIndex := -1
	lastPlayerIndex := -1

	if len(h.gameState.PlayerOrder) > 0 {
		for i := 1; i <= len(h.gameState.PlayerOrder); i++ {
			idx := (h.gameState.DealerIndex + i) % len(h.gameState.PlayerOrder)
			playerID := h.gameState.PlayerOrder[idx]
			if p, ok := h.gameState.Players[playerID]; ok && p.IsInHand && p.IsConnected && p.Chips > 0 {
				firstPlayerIndex = idx
				break
			}
		}

		for i := 0; i < len(h.gameState.PlayerOrder); i++ {
			idx := (h.gameState.DealerIndex - i + len(h.gameState.PlayerOrder)) % len(h.gameState.PlayerOrder)
			playerID := h.gameState.PlayerOrder[idx]
			if p, ok := h.gameState.Players[playerID]; ok && p.IsInHand && p.IsConnected && p.Chips > 0 {
				lastPlayerIndex = idx
				break
			}
		}
	}

	h.gameState.CurrentTurnIndex = firstPlayerIndex
	if firstPlayerIndex != -1 {
		h.gameState.actionToPlayerID = h.gameState.PlayerOrder[lastPlayerIndex]
	} else {
		h.gameState.actionToPlayerID = ""
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
	}

	h.gameState.LastBet = 0

	if h.gameState.CurrentTurnIndex == -1 {
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
			continue
		}
		switch msg.Type {
		case "player_ready":
			var payload struct {
				IsReady bool `json:"isReady"`
			}
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
