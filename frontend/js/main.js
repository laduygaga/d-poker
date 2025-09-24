class GameScene extends Phaser.Scene {
    constructor() {
        super({ key: 'GameScene' });
        this.playerObjects = {}; // Lưu trữ các đối tượng game của người chơi
        this.communityCardObjects = []; // Lưu các lá bài chung
        this.isReady = false;
        this.myId = null; // ID của client này
        this.gameState = {}; // Giữ trạng thái game mới nhất
    }

    create() {
        this.add.text(400, 30, 'D-Poker', { fontSize: '40px', fill: '#fff' }).setOrigin(0.5);
        this.statusText = this.add.text(400, 70, 'Connecting...', { fontSize: '24px', fill: '#fff' }).setOrigin(0.5);
        this.potText = this.add.text(400, 280, 'Pot: 0', { fontSize: '28px', fill: '#ffc300' }).setOrigin(0.5);
        this.communityCardContainer = this.add.container(400, 220);

        // --- Nút Ready ---
        this.readyButton = this.add.text(700, 550, 'Ready', {
            fontSize: '28px', fill: '#0f0', backgroundColor: '#555',
            padding: { left: 15, right: 15, top: 10, bottom: 10 }
        }).setOrigin(0.5).setInteractive();

        this.readyButton.on('pointerdown', () => {
            this.isReady = !this.isReady;
            this.updateReadyButton();
            this.socket.send(JSON.stringify({ type: 'player_ready', payload: { isReady: this.isReady } }));
        });
        
        // --- Các nút hành động trong game ---
        this.actionContainer = this.add.container(400, 550);
        const foldButton = this.add.text(-150, 0, 'Fold', { fontSize: '28px', fill: '#ff5733', backgroundColor: '#555', padding: {x: 10, y: 5} }).setOrigin(0.5).setInteractive();
        const callButton = this.add.text(0, 0, 'Call', { fontSize: '28px', fill: '#33ff57', backgroundColor: '#555', padding: {x: 10, y: 5} }).setOrigin(0.5).setInteractive();
        const raiseButton = this.add.text(150, 0, 'Raise', { fontSize: '28px', fill: '#3375ff', backgroundColor: '#555', padding: {x: 10, y: 5} }).setOrigin(0.5).setInteractive();
        
        this.actionContainer.add([foldButton, callButton, raiseButton]);
        this.actionContainer.setVisible(false); // Ẩn đi lúc đầu

        foldButton.on('pointerdown', () => this.sendPlayerAction('fold'));
        callButton.on('pointerdown', () => {
            const action = this.callButton.text === 'Check' ? 'check' : 'call';
            this.sendPlayerAction(action);
        });
        raiseButton.on('pointerdown', () => {
            const me = this.gameState.players[this.myId];
            const minRaise = this.gameState.lastBet * 2;
            const raiseAmount = parseInt(prompt(`Raise amount (min ${minRaise}):`, minRaise), 10);
             if (!isNaN(raiseAmount) && raiseAmount >= me.bet + this.gameState.lastBet) {
                this.sendPlayerAction('raise', { amount: raiseAmount });
            }
        });
        this.callButton = callButton;


        // --- Logic WebSocket ---
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
                    this.gameState = message.payload; // Lưu lại state để tham chiếu
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
        this.readyButton.setVisible(!state.gameStarted);
        if (state.gameStarted && this.isReady) {
            this.isReady = false;
            this.updateReadyButton();
        }
        
        this.potText.setText(`Pot: ${state.pot || 0}`);

        // Cập nhật bài chung
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

        const allPlayerIds = Object.keys(state.players);

        // Xóa các đối tượng người chơi không còn trong state
        for (const existingId in this.playerObjects) {
            if (!allPlayerIds.includes(existingId)) {
                this.playerObjects[existingId].destroy();
                delete this.playerObjects[existingId];
            }
        }

        // **** LOGIC HIỂN THỊ ĐÃ SỬA LỖI ****
        const otherPlayerIds = allPlayerIds.filter(id => id !== this.myId);
        
        // Vị trí hiển thị cố định
        const otherPositions = [
            { x: 100, y: 250 }, // Trái
            { x: 250, y: 100 }, // Trái trên
            { x: 550, y: 100 }, // Phải trên
            { x: 700, y: 250 }, // Phải
            { x: 400, y: 400 }, // Đối diện (nếu > 5 người)
        ];
        const myPosition = { x: 400, y: 500 }; // Vị trí của mình

        // Hiển thị những người chơi khác
        otherPlayerIds.forEach((playerId, index) => {
            const player = state.players[playerId];
            const pos = otherPositions[index];
            this.renderPlayer(player, pos, state);
        });

        // Hiển thị chính mình
        if (state.players[this.myId]) {
            this.renderPlayer(state.players[this.myId], myPosition, state);
        }
        
        // Hiển thị nút hành động nếu đến lượt
        const myTurn = state.gameStarted && state.playerOrder && state.playerOrder[state.currentTurnIndex] === this.myId;
        this.actionContainer.setVisible(myTurn);
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
        } else if (player.isReady) {
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

        // Xóa bài cũ (nếu có)
        if (playerObj.cards) {
            playerObj.cards.forEach(c => c.destroy());
        }
        playerObj.cards = [];

        // Vẽ bài mới
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
