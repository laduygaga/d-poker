# D-Poker - Complete Texas Hold'em Poker Game

A real-time multiplayer Texas Hold'em poker game built with Go backend and JavaScript frontend using Phaser.js.

## Features

### ✅ Completed Features

#### Core Gameplay
- **Texas Hold'em Rules**: Complete implementation of Texas Hold'em poker rules
- **Real-time Multiplayer**: WebSocket-based real-time gameplay for multiple players
- **Hand Evaluation**: Comprehensive hand ranking system (High Card to Royal Flush)
- **All Game Phases**: Pre-flop, Flop, Turn, River, and Showdown
- **Betting Actions**: Fold, Check, Call, Raise with proper validation
- **All-in Support**: Players can go all-in when they don't have enough chips
- **Pot Management**: Proper pot calculation and distribution to winners
- **Side Pots**: Support for side pots when players are all-in with different amounts

#### Player Management
- **Custom Player Names**: Players can set and display custom names
- **Player Status Indicators**: 
  - Dealer (D), Small Blind (SB), Big Blind (BB) positions
  - ALL-IN status
  - FOLDED status
  - Ready status
- **Chip Management**: Starting chips (1000), proper chip deduction and awarding
- **Player Elimination**: Players are eliminated when they run out of chips
- **Reconnection Handling**: Players can disconnect and reconnect

#### User Interface
- **Modern Web UI**: Built with Phaser.js for smooth gameplay experience
- **Real-time Chat System**: Players can chat during gameplay
- **Visual Card Display**: Cards shown face-up for players and community cards
- **Player Positioning**: Players arranged around a virtual table
- **Action Buttons**: Context-aware Fold/Check/Call/Raise buttons
- **Raise Amount Input**: Players can specify custom raise amounts
- **Game Status Display**: Current pot, player chips, bets, and game phase
- **Color-coded Players**: Different colors for current turn, all-in, and folded players

#### Performance & Reliability
- **Memory Management**: Efficient card deck pooling and object reuse
- **Performance Monitoring**: Built-in metrics logging system
- **Connection Management**: Automatic reconnection with exponential backoff
- **Error Handling**: Robust error handling throughout the application

### 🎮 How to Play

1. **Start the Server**:
   ```bash
   cd backend
   go run .
   ```

2. **Open the Game**: Navigate to `http://localhost:8080` in your browser

3. **Set Your Name**: Click "Click here to set name" and enter your name

4. **Join the Game**: Click the "Ready" button when you're ready to play

5. **Gameplay**:
   - Wait for at least 2 players to be ready
   - Game starts automatically with blinds posted
   - Use Fold/Check/Call/Raise buttons when it's your turn
   - For raises, enter the total amount you want to bet
   - Chat with other players using the chat system

### 🎯 Game Rules

#### Betting Rounds
1. **Pre-flop**: Each player gets 2 hole cards, betting starts with player after big blind
2. **Flop**: 3 community cards dealt, betting starts with small blind
3. **Turn**: 1 additional community card, betting continues
4. **River**: Final community card, last betting round
5. **Showdown**: Best 5-card hand from 7 available cards wins

#### Blinds
- Small Blind: 10 chips
- Big Blind: 20 chips
- Minimum Raise: Equal to the big blind amount

#### Hand Rankings (High to Low)
1. Royal Flush
2. Straight Flush  
3. Four of a Kind
4. Full House
5. Flush
6. Straight
7. Three of a Kind
8. Two Pair
9. One Pair
10. High Card

### 🛠️ Technical Architecture

#### Backend (Go)
- **WebSocket Server**: Real-time bidirectional communication
- **Game Logic**: Complete poker game state management
- **Hand Evaluation**: Efficient algorithm for determining winning hands
- **Memory Pooling**: Optimized memory usage with object pools
- **Concurrent Safety**: Thread-safe game state management with mutexes

#### Frontend (JavaScript + Phaser.js)
- **Game Rendering**: Hardware-accelerated 2D rendering
- **UI Management**: Dynamic interface updates
- **WebSocket Client**: Automatic reconnection and message handling
- **Object Pooling**: Efficient card and UI object reuse

### 📁 Project Structure

```
d-poker/
├── backend/
│   ├── main.go          # Main server and game logic
│   ├── metrics.go       # Performance monitoring
│   ├── go.mod          # Go module dependencies
│   └── go.sum          # Dependency checksums
├── frontend/
│   ├── index.html      # Game entry point
│   └── js/
│       └── main.js     # Game client logic
└── README.md           # This file
```

### 🚀 Advanced Features

#### Performance Monitoring
The server includes built-in performance monitoring that logs:
- Active connections and players
- Memory usage (Alloc, TotalAlloc, Sys)
- Garbage collection statistics
- Metrics logged every 30 seconds

#### Robust Connection Management
- Automatic client reconnection with exponential backoff
- Graceful handling of player disconnections during games
- WebSocket ping/pong for connection health monitoring

#### Scalable Architecture
- Clean separation between game logic and networking
- Efficient memory management with object pooling
- Thread-safe concurrent access to game state

### 🔧 Development

#### Requirements
- Go 1.19+ 
- Modern web browser with WebSocket support

#### Building
```bash
cd backend
go build .
```

#### Running Tests
```bash
cd backend  
go test .
```

### 🐛 Known Issues & Future Enhancements

#### Potential Improvements
- Tournament mode with increasing blinds
- Multi-table support
- Player statistics and hand history
- Spectator mode
- Mobile-responsive design
- Database persistence for player accounts
- Anti-cheat measures
- Configurable game settings (starting chips, blind levels)

#### Performance Optimizations
- Redis for game state persistence
- Load balancing for multiple game instances  
- CDN for static assets
- WebRTC for peer-to-peer connections

### 📄 License

This project is open source and available under the MIT License.

### 🤝 Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues for bugs and feature requests.

---

**Enjoy playing D-Poker!** 🃏♠️♥️♦️♣️