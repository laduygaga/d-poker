class GameScene extends Phaser.Scene {
    constructor() {
        super({ key: 'GameScene' });
        this.playerObjects = {}; // Lưu trữ các đối tượng game của người chơi
        this.isReady = false;
        this.myId = null; // ID của client này
    }

    create() {
        this.add.text(400, 30, 'D-Poker', { fontSize: '40px', fill: '#fff' }).setOrigin(0.5);
        this.statusText = this.add.text(400, 70, 'Connecting...', { fontSize: '24px', fill: '#fff' }).setOrigin(0.5);

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
                    console.log(`My ID is: ${this.myId}`);
                    break;
                case 'game_state':
                    this.updateGameState(message.payload);
                    break;
            }
        };
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

        for (const playerId in this.playerObjects) {
            if (!state.players[playerId]) {
                this.playerObjects[playerId].text.destroy();
                if (this.playerObjects[playerId].cards) {
                    this.playerObjects[playerId].cards.forEach(c => c.destroy());
                }
                delete this.playerObjects[playerId];
            }
        }

        let yPos = 150;
        for (const playerId in state.players) {
            const player = state.players[playerId];
            
            const readyText = player.isReady ? ' (Ready)' : '';
            const displayText = `Player: ${player.id}${readyText}`;
            if (this.playerObjects[playerId]) {
                this.playerObjects[playerId].text.setText(displayText);
            } else {
                this.playerObjects[playerId] = { 
                    text: this.add.text(50, yPos, displayText, { fontSize: '18px', fill: '#fff' })
                };
            }

            if (this.playerObjects[playerId].cards) {
                this.playerObjects[playerId].cards.forEach(c => c.destroy());
            }
            this.playerObjects[playerId].cards = [];

            if (player.hand && player.hand.length > 0) {
                let cardX = 300;
                player.hand.forEach(card => {
                    const cardText = (player.id === this.myId) 
                        ? `${card.rank} ${card.suit}` 
                        : '[Card]';
                    
                    const cardObj = this.add.text(cardX, this.playerObjects[playerId].text.y, cardText, { 
                        fontSize: '18px', fill: '#000', backgroundColor: '#fff', padding: 5 
                    });
                    this.playerObjects[playerId].cards.push(cardObj);
                    cardX += 100;
                });
            }
            yPos += 40;
        }
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
