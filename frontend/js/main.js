class GameScene extends Phaser.Scene {
    constructor() {
        super({ key: 'GameScene' });
        this.playerObjects = {}; // Stores game objects for players
        this.communityCardObjects = []; // Stores community card objects
        this.cardPool = []; // Pool for reusing card objects
        this.isReady = false; // Local flag for immediate UI feedback
        this.myId = null; // ID for this client
        this.gameState = {}; // Holds the latest game state from the server
        this.reconnectAttempts = 0; // Track reconnection attempts
        this.playerName = ''; // Player's chosen name
    }

    create() {
        this.add.text(400, 30, 'D-Poker', { fontSize: '40px', fill: '#fff' }).setOrigin(0.5);
        this.statusText = this.add.text(400, 70, 'Connecting...', { fontSize: '24px', fill: '#fff' }).setOrigin(0.5);
        this.potText = this.add.text(400, 280, 'Pot: 0', { fontSize: '28px', fill: '#ffc300' }).setOrigin(0.5);
        this.communityCardContainer = this.add.container(400, 220);

        // Player name input
        this.createNameInput();

        // --- New Game Result Text ---
        this.gameResultText = this.add.text(400, 350, '', {
            fontSize: '32px',
            fill: '#00ff00',
            backgroundColor: 'rgba(0,0,0,0.7)',
            padding: { x: 20, y: 10 },
            align: 'center'
        }).setOrigin(0.5).setDepth(100).setVisible(false);

        // Chat system
        this.createChatSystem();

        // --- Ready Button ---
        this.readyButton = this.add.text(700, 550, 'Ready', {
            fontSize: '28px', fill: '#0f0', backgroundColor: '#555',
            padding: { left: 15, right: 15, top: 10, bottom: 10 }
        }).setOrigin(0.5).setInteractive();

        this.readyButton.on('pointerdown', () => {
            if (!this.playerName) {
                this.statusText.setText('Please enter your name first!');
                return;
            }
            this.isReady = !this.isReady;
            this.updateReadyButton();
            this.sendMessage({ type: 'player_ready', payload: { isReady: this.isReady } });
        });
        
        // --- In-Game Action Buttons ---
        this.createActionButtons();

        // --- WebSocket Logic with Reconnection ---
        this.connectToServer();
    }
    
    createNameInput() {
        this.nameText = this.add.text(100, 100, 'Enter your name:', { fontSize: '18px', fill: '#fff' });
        this.nameInputPrompt = this.add.text(100, 130, 'Click here to set name', { 
            fontSize: '16px', 
            fill: '#ffc300', 
            backgroundColor: '#333',
            padding: { x: 10, y: 5 }
        }).setInteractive();
        
        this.nameInputPrompt.on('pointerdown', () => {
            const name = prompt('Enter your name:', this.playerName || 'Player');
            if (name && name.trim()) {
                this.playerName = name.trim();
                this.nameInputPrompt.setText(`Name: ${this.playerName}`);
                this.sendMessage({ type: 'player_join', payload: { name: this.playerName } });
            }
        });
    }
    
    createChatSystem() {
        // Chat display area
        this.chatContainer = this.add.container(50, 300);
        this.chatBackground = this.add.rectangle(0, 0, 300, 200, 0x000000, 0.7).setOrigin(0);
        this.chatContainer.add(this.chatBackground);
        
        this.chatMessages = [];
        this.maxChatMessages = 8;
        
        // Chat input
        this.chatInputPrompt = this.add.text(50, 520, 'Click to chat', { 
            fontSize: '14px', 
            fill: '#aaa', 
            backgroundColor: '#333',
            padding: { x: 5, y: 3 }
        }).setInteractive();
        
        this.chatInputPrompt.on('pointerdown', () => {
            const message = prompt('Enter message:');
            if (message && message.trim()) {
                this.sendMessage({ type: 'chat_message', payload: { message: message.trim() } });
            }
        });
    }
    
    createActionButtons() {
        this.actionContainer = this.add.container(400, 550);
        const foldButton = this.add.text(-150, 0, 'Fold', { 
            fontSize: '24px', 
            fill: '#ff5733', 
            backgroundColor: '#555', 
            padding: {x: 10, y: 5} 
        }).setOrigin(0.5).setInteractive();
        
        const callButton = this.add.text(0, 0, 'Call', { 
            fontSize: '24px', 
            fill: '#33ff57', 
            backgroundColor: '#555', 
            padding: {x: 10, y: 5} 
        }).setOrigin(0.5).setInteractive();
        
        const raiseButton = this.add.text(150, 0, 'Raise', { 
            fontSize: '24px', 
            fill: '#3375ff', 
            backgroundColor: '#555', 
            padding: {x: 10, y: 5} 
        }).setOrigin(0.5).setInteractive();
        
        this.actionContainer.add([foldButton, callButton, raiseButton]);
        this.actionContainer.setVisible(false);

        foldButton.on('pointerdown', () => this.sendPlayerAction('fold'));
        callButton.on('pointerdown', () => {
            const action = callButton.text === 'Check' ? 'check' : 'call';
            this.sendPlayerAction(action);
        });
        raiseButton.on('pointerdown', () => {
            if (!this.gameState.players || !this.gameState.players[this.myId]) return;
            
            const me = this.gameState.players[this.myId];
            const callAmount = this.gameState.lastBet - me.bet;
            const minRaise = this.gameState.lastBet + (this.gameState.minRaise || 20);
            const maxRaise = me.chips + me.bet; // Total chips available
            
            const raiseAmount = parseInt(prompt(`Raise to amount (min ${minRaise}, max ${maxRaise}):`, minRaise), 10);
            if (!isNaN(raiseAmount) && raiseAmount >= minRaise && raiseAmount <= maxRaise) {
                this.sendPlayerAction('raise', { amount: raiseAmount });
            } else {
                alert(`Invalid raise amount. Must be between ${minRaise} and ${maxRaise}.`);
            }
        });
        
        this.callButton = callButton;
    }
    
    connectToServer() {
        this.statusText.setText('Connecting...');
        this.socket = new WebSocket('ws://localhost:8080/ws');
        
        this.socket.onopen = () => {
            this.statusText.setText('Connected!');
            this.reconnectAttempts = 0;
        };
        
        this.socket.onerror = (error) => {
            this.statusText.setText('Connection Error!');
            console.error('WebSocket error:', error);
        };
        
        this.socket.onclose = (event) => {
            this.statusText.setText('Connection Closed. Reconnecting...');
            this.scheduleReconnect();
        };

        this.socket.onmessage = (event) => {
            const message = JSON.parse(event.data);
            switch (message.type) {
                case 'your_id':
                    this.myId = message.payload.id;
                    break;
                case 'game_state':
                    this.gameState = message.payload;
                    this.updateGameState(this.gameState);
                    break;
            }
        };
    }
    
    scheduleReconnect() {
        if (!this.reconnectAttempts) this.reconnectAttempts = 0;
        if (this.reconnectAttempts < 10) {
            const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000); // Exponential backoff
            this.reconnectAttempts++;
            setTimeout(() => this.connectToServer(), delay);
        } else {
            this.statusText.setText('Connection Failed. Please refresh the page.');
        }
    }
    
    sendMessage(message) {
        if (this.socket && this.socket.readyState === WebSocket.OPEN) {
            this.socket.send(JSON.stringify(message));
        } else {
            console.warn('WebSocket not connected, message queued');
            // Could implement message queuing here
        }
    }
    
    sendPlayerAction(action, payload = {}) {
        this.sendMessage({ type: 'player_action', payload: { action, ...payload } });
        this.actionContainer.setVisible(false);
    }

    updateReadyButton() {
        this.readyButton.setText(this.isReady ? 'Cancel' : 'Ready');
        this.readyButton.setStyle({ fill: this.isReady ? '#ff0' : '#0f0' });
    }

    getCardFromPool() {
        if (this.cardPool.length > 0) {
            return this.cardPool.pop();
        }
        return this.add.text(0, 0, '', { 
            fontSize: '24px', fill: '#000', backgroundColor: '#fff', padding: 8 
        }).setOrigin(0.5);
    }
    
    returnCardToPool(card) {
        card.setVisible(false);
        this.cardPool.push(card);
    }

    updateGameState(state) {
        if (state.playerReady) {
            this.isReady = state.playerReady[this.myId] || false;
        }

        this.updateReadyButton();
        this.readyButton.setVisible(!state.gameStarted);
        
        this.potText.setText(`Pot: ${state.pot || 0}`);

        // Return old cards to pool instead of destroying
        this.communityCardObjects.forEach(card => this.returnCardToPool(card));
        this.communityCardObjects = [];
        if (state.communityCards) {
            let startX = -(state.communityCards.length - 1) * 35;
            state.communityCards.forEach((card, index) => {
                const cardText = `${card.rank}${card.suit}`;
                const cardObj = this.getCardFromPool();
                cardObj.setText(cardText);
                cardObj.setPosition(startX + index * 70, 0);
                cardObj.setVisible(true);
                this.communityCardContainer.add(cardObj);
                this.communityCardObjects.push(cardObj);
            });
        }

        const allPlayerIds = state.players ? Object.keys(state.players) : [];

        for (const existingId in this.playerObjects) {
            if (!allPlayerIds.includes(existingId)) {
                this.playerObjects[existingId].destroy();
                delete this.playerObjects[existingId];
            }
        }
        
        const otherPlayerIds = allPlayerIds.filter(id => id !== this.myId);
        
        const otherPositions = [
            { x: 100, y: 250 }, { x: 250, y: 100 },
            { x: 550, y: 100 }, { x: 700, y: 250 },
            { x: 400, y: 400 },
        ];
        const myPosition = { x: 400, y: 500 };

        allPlayerIds.forEach(id => {
            const player = state.players[id];
            if (!player.isConnected) {
                if(this.playerObjects[id]) {
                    this.playerObjects[id].destroy();
                    delete this.playerObjects[id];
                }
                return;
            }
            const isMe = id === this.myId;
            const pos = isMe ? myPosition : otherPositions[otherPlayerIds.indexOf(id)];
            this.renderPlayer(player, pos, state);
        });
        
        const myTurn = state.gameStarted && state.playerOrder && state.playerOrder[state.currentTurnIndex] === this.myId;
        
        // --- Game Phase Logic ---
        if (state.gamePhase === 'showdown') {
            this.actionContainer.setVisible(false);
            this.gameResultText.setText(state.winningHandDesc || 'Showdown!');
            this.gameResultText.setVisible(true);
        } else {
            // Hide result text if not in showdown
            this.gameResultText.setVisible(false);
            // Show actions only if it's my turn
            this.actionContainer.setVisible(myTurn);
        }

        if(myTurn) {
            const me = state.players[this.myId];
            if (me.bet >= state.lastBet) {
                this.callButton.setText('Check');
            } else {
                this.callButton.setText(`Call ${state.lastBet - me.bet}`);
            }
        }

        // Update chat messages
        this.updateChatMessages(state.chatMessages);
    }

    updateChatMessages(messages) {
        if (!messages) return;
        
        // Clear old messages
        this.chatMessages.forEach(msg => msg.destroy());
        this.chatMessages = [];
        
        // Show last 8 messages
        const recentMessages = messages.slice(-this.maxChatMessages);
        recentMessages.forEach((msg, index) => {
            let displayText;
            let color = '#fff';
            
            if (msg.playerId === 'system') {
                displayText = `*** ${msg.message} ***`;
                color = '#ffff00'; // Yellow for system messages
            } else {
                const playerName = this.gameState.players && this.gameState.players[msg.playerId] ? 
                                 this.gameState.players[msg.playerId].name : 'Player';
                displayText = `${playerName}: ${msg.message}`;
            }
            
            const text = this.add.text(10, 10 + index * 20, displayText, {
                fontSize: '12px',
                fill: color,
                wordWrap: { width: 280 }
            });
            this.chatContainer.add(text);
            this.chatMessages.push(text);
        });
    }

    renderPlayer(player, position, state) {
        if (!player || !position) return;

        const playerId = player.id;
        const isMyTurn = state.gameStarted && state.playerOrder && state.playerOrder[state.currentTurnIndex] === playerId;

        let statusIcons = '';
        if (state.gameStarted && state.playerOrder) {
            const pIndex = state.playerOrder.indexOf(playerId);
            if (pIndex !== -1) {
                if (pIndex === state.dealerIndex) statusIcons += ' (D)';
                if (pIndex === (state.dealerIndex + 1) % state.playerOrder.length) statusIcons += ' (SB)';
                if (pIndex === (state.dealerIndex + 2) % state.playerOrder.length) statusIcons += ' (BB)';
            }
            if (player.isAllIn) statusIcons += ' (ALL-IN)';
            if (!player.isInHand) statusIcons += ' (FOLDED)';
        } else if (state.playerReady && state.playerReady[playerId]) {
             statusIcons += ' (Ready)';
        }
        
        const playerName = player.name || player.id.substring(0, 8);
        const displayText = `${playerName}${statusIcons}\nChips: ${player.chips}\nBet: ${player.bet || 0}`;

        let playerObj = this.playerObjects[playerId];

        if (!playerObj) {
            playerObj = this.add.container(position.x, position.y);
            const text = this.add.text(0, 0, displayText, { fontSize: '16px', fill: '#fff', align: 'center' }).setOrigin(0.5);
            playerObj.add(text);
            playerObj.text = text;
            this.playerObjects[playerId] = playerObj;
        } else {
            playerObj.text.setText(displayText);
            playerObj.setPosition(position.x, position.y);
        }

        // Color coding: yellow for current turn, red for all-in, gray for folded
        let textColor = '#fff';
        if (isMyTurn) textColor = '#ffff00';
        else if (player.isAllIn) textColor = '#ff6666';
        else if (!player.isInHand && state.gameStarted) textColor = '#888888';
        
        playerObj.text.setStyle({ fill: textColor });

        if (playerObj.cards) {
            playerObj.cards.forEach(c => c.destroy());
        }
        playerObj.cards = [];

        if (player.hand && player.hand.length > 0) {
            player.hand.forEach((card, i) => {
                const cardText = (player.id === this.myId || state.gamePhase === 'showdown') ? `${card.rank}${card.suit}` : '[]';
                const cardBgColor = player.isInHand ? '#fff' : '#888';
                const cardObj = this.add.text((i - 0.5) * 60, -50, cardText, { 
                    fontSize: '20px', fill: '#000', backgroundColor: cardBgColor, padding: 5 
                }).setOrigin(0.5);
                playerObj.add(cardObj);
                playerObj.cards.push(cardObj);
            });
        }
        
        playerObj.alpha = player.isInHand ? 1.0 : 0.5;
    }
}

const config = {
    type: Phaser.AUTO,
    width: 800,
    height: 600,
    scene: [GameScene],
    backgroundColor: '#2d2d2d',
    scale: {
        mode: Phaser.Scale.FIT,
        autoCenter: Phaser.Scale.CENTER_BOTH
    }
};

const game = new Phaser.Game(config);
