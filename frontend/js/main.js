class GameScene extends Phaser.Scene {
    constructor() {
        super({ key: 'GameScene' });
        this.playerObjects = {}; // Stores game objects for players
        this.communityCardObjects = []; // Stores community card objects
        this.isReady = false; // Local flag for immediate UI feedback
        this.myId = null; // ID for this client
        this.gameState = {}; // Holds the latest game state from the server
    }

    create() {
        this.add.text(400, 30, 'D-Poker', { fontSize: '40px', fill: '#fff' }).setOrigin(0.5);
        this.statusText = this.add.text(400, 70, 'Connecting...', { fontSize: '24px', fill: '#fff' }).setOrigin(0.5);
        this.potText = this.add.text(400, 280, 'Pot: 0', { fontSize: '28px', fill: '#ffc300' }).setOrigin(0.5);
        this.communityCardContainer = this.add.container(400, 220);

        // --- New Game Result Text ---
        // This will display the winner and is hidden by default.
        this.gameResultText = this.add.text(400, 350, '', {
            fontSize: '32px',
            fill: '#00ff00',
            backgroundColor: 'rgba(0,0,0,0.7)',
            padding: { x: 20, y: 10 },
            align: 'center'
        }).setOrigin(0.5).setDepth(100).setVisible(false);


        // --- Ready Button ---
        this.readyButton = this.add.text(700, 550, 'Ready', {
            fontSize: '28px', fill: '#0f0', backgroundColor: '#555',
            padding: { left: 15, right: 15, top: 10, bottom: 10 }
        }).setOrigin(0.5).setInteractive();

        this.readyButton.on('pointerdown', () => {
            this.isReady = !this.isReady;
            this.updateReadyButton();
            this.socket.send(JSON.stringify({ type: 'player_ready', payload: { isReady: this.isReady } }));
        });
        
        // --- In-Game Action Buttons ---
        this.actionContainer = this.add.container(400, 550);
        const foldButton = this.add.text(-150, 0, 'Fold', { fontSize: '28px', fill: '#ff5733', backgroundColor: '#555', padding: {x: 10, y: 5} }).setOrigin(0.5).setInteractive();
        const callButton = this.add.text(0, 0, 'Call', { fontSize: '28px', fill: '#33ff57', backgroundColor: '#555', padding: {x: 10, y: 5} }).setOrigin(0.5).setInteractive();
        const raiseButton = this.add.text(150, 0, 'Raise', { fontSize: '28px', fill: '#3375ff', backgroundColor: '#555', padding: {x: 10, y: 5} }).setOrigin(0.5).setInteractive();
        
        this.actionContainer.add([foldButton, callButton, raiseButton]);
        this.actionContainer.setVisible(false);

        foldButton.on('pointerdown', () => this.sendPlayerAction('fold'));
        callButton.on('pointerdown', () => {
            const action = this.callButton.text === 'Check' ? 'check' : 'call';
            this.sendPlayerAction(action);
        });
        raiseButton.on('pointerdown', () => {
            const me = this.gameState.players[this.myId];
            const minRaise = this.gameState.lastBet * 2 || 20; // Default to Big Blind if no bet
            const raiseAmount = parseInt(prompt(`Raise amount (min ${minRaise}):`, minRaise), 10);
            if (!isNaN(raiseAmount) && raiseAmount >= minRaise && raiseAmount <= me.chips) {
                this.sendPlayerAction('raise', { amount: raiseAmount });
            } else {
                alert(`Invalid raise amount. Must be between ${minRaise} and your chip count of ${me.chips}.`);
            }
        });
        this.callButton = callButton;


        // --- WebSocket Logic ---
        this.socket = new WebSocket('ws://localhost:8080/ws');
        this.socket.onopen = () => this.statusText.setText('Connected!');
        this.socket.onerror = (error) => this.statusText.setText('Connection Error!');
        this.socket.onclose = () => this.statusText.setText('Connection Closed.');

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
    
    sendPlayerAction(action, payload = {}) {
        this.socket.send(JSON.stringify({ type: 'player_action', payload: { action, ...payload } }));
        this.actionContainer.setVisible(false);
    }

    updateReadyButton() {
        this.readyButton.setText(this.isReady ? 'Cancel' : 'Ready');
        this.readyButton.setStyle({ fill: this.isReady ? '#ff0' : '#0f0' });
    }

    updateGameState(state) {
        if (state.playerReady) {
            this.isReady = state.playerReady[this.myId] || false;
        }

        this.updateReadyButton();
        this.readyButton.setVisible(!state.gameStarted);
        
        this.potText.setText(`Pot: ${state.pot || 0}`);

        this.communityCardObjects.forEach(card => card.destroy());
        this.communityCardObjects = [];
        if (state.communityCards) {
            let startX = -(state.communityCards.length - 1) * 35;
            state.communityCards.forEach((card, index) => {
                const cardText = `${card.rank}${card.suit}`;
                const cardObj = this.add.text(startX + index * 70, 0, cardText, { 
                    fontSize: '24px', fill: '#000', backgroundColor: '#fff', padding: 8 
                }).setOrigin(0.5);
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
        } else if (state.playerReady && state.playerReady[playerId]) {
             statusIcons += ' (Ready)';
        }
        
        const displayText = `${player.id.substring(0, 5)}${statusIcons}\nChips: ${player.chips}\nBet: ${player.bet || 0}`;

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

        playerObj.text.setStyle({ fill: isMyTurn ? '#ffff00' : '#fff' });

        if (playerObj.cards) {
            playerObj.cards.forEach(c => c.destroy());
        }
        playerObj.cards = [];

        if (player.hand && player.hand.length > 0) {
            player.hand.forEach((card, i) => {
                const cardText = (player.id === this.myId || state.gamePhase === 'showdown') ? `${card.rank}${card.suit}` : '[]';
                const cardObj = this.add.text((i - 0.5) * 60, -50, cardText, { 
                    fontSize: '20px', fill: '#000', backgroundColor: player.isInHand ? '#fff' : '#888', padding: 5 
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
