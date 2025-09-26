class GameScene extends Phaser.Scene {
    constructor() {
        super({ key: 'GameScene' });
        this.playerObjects = {};
        this.communityCardObjects = [];
        this.isReady = false;
        this.myId = null;
        this.gameState = {};
        this.reconnectAttempts = 0;
        this.playerName = '';
        this.dealerChip = null;
    }

    preload() {
        this.createCardTextures();
        this.createChipTextures();
        this.createTableTexture();
        this.createUITextures();
    }

    create() {
        this.setupTable();
        this.setupUI();
        this.setupAnimations();
        this.initializeUIElements();
        this.setupDebugListener();
        this.connectToServer();
    }

    createCardTextures() {
        const suits = ['♠', '♥', '♦', '♣'];
        const ranks = ['2', '3', '4', '5', '6', '7', '8', '9', '10', 'J', 'Q', 'K', 'A'];
        
        this.createCardBack();
        
        suits.forEach(suit => {
            ranks.forEach(rank => {
                this.createCard(rank, suit);
            });
        });
    }

    createCardBack() {
        const graphics = this.add.graphics();
        const width = 90;
        const height = 130;
        
        graphics.fillGradientStyle(0x1a1a2e, 0x16213e, 0x0f4c75, 0x3282b8);
        graphics.fillRoundedRect(0, 0, width, height, 10);
        
        graphics.lineStyle(3, 0xffd700);
        graphics.strokeRoundedRect(1.5, 1.5, width-3, height-3, 10);
        
        graphics.lineStyle(2, 0x4a90e2);
        graphics.strokeRoundedRect(10, 10, width-20, height-20, 5);
        
        graphics.lineStyle(1, 0xffd700);
        const centerX = width / 2;
        const centerY = height / 2;
        for (let i = 0; i < 8; i++) {
            const angle = (i * Math.PI * 2) / 8;
            const x1 = centerX + Math.cos(angle) * 15;
            const y1 = centerY + Math.sin(angle) * 15;
            const x2 = centerX + Math.cos(angle + Math.PI) * 15;
            const y2 = centerY + Math.sin(angle + Math.PI) * 15;
            graphics.lineBetween(x1, y1, x2, y2);
        }
        
        graphics.generateTexture('card-back', width, height);
        graphics.destroy();
    }

    createCard(rank, suit) {
        const graphics = this.add.graphics();
        const width = 90;
        const height = 130;
        
        graphics.fillStyle(0xffffff);
        graphics.fillRoundedRect(0, 0, width, height, 8);
        
        graphics.lineStyle(2, 0x333333);
        graphics.strokeRoundedRect(1, 1, width-2, height-2, 8);
        
        const isRed = suit === '♥' || suit === '♦';
        
        const style = {
            fontSize: rank === '10' ? '16px' : '18px',
            fill: isRed ? '#ff0000' : '#000000',
            fontFamily: 'Arial Black',
            fontWeight: 'bold'
        };
        
        const suitStyle = {
            fontSize: '24px',
            fill: isRed ? '#ff0000' : '#000000',
            fontFamily: 'Arial'
        };
        
        const rankText1 = this.add.text(8, 8, rank, style).setOrigin(0, 0);
        const suitText1 = this.add.text(8, rank === '10' ? 28 : 30, suit, suitStyle).setOrigin(0, 0);
        
        const rankText2 = this.add.text(width-8, height-25, rank, style).setOrigin(1, 1).setRotation(Math.PI);
        const suitText2 = this.add.text(width-8, height-45, suit, suitStyle).setOrigin(1, 1).setRotation(Math.PI);
        
        const centerSuitStyle = {
            fontSize: rank === 'A' ? '36px' : rank === 'K' || rank === 'Q' || rank === 'J' ? '32px' : '28px',
            fill: isRed ? '#ff0000' : '#000000',
            fontFamily: 'Arial'
        };
        this.add.text(width/2, height/2, suit, centerSuitStyle).setOrigin(0.5);
        
        graphics.generateTexture(`card-${rank}-${suit}`, width, height);
        
        rankText1.destroy();
        suitText1.destroy();
        rankText2.destroy();
        suitText2.destroy();
        graphics.destroy();
    }

    createChipTextures() {
        const colors = [
            { value: 1, color: 0xffffff, accent: 0x333333 },
            { value: 5, color: 0xff0000, accent: 0x880000 },
            { value: 10, color: 0x0080ff, accent: 0x004080 },
            { value: 25, color: 0x008000, accent: 0x004000 },
            { value: 100, color: 0x000000, accent: 0x666666 },
            { value: 500, color: 0x800080, accent: 0x400040 },
            { value: 1000, color: 0xffa500, accent: 0x804000 }
        ];

        colors.forEach(chip => {
            const graphics = this.add.graphics();
            const radius = 25;
            
            graphics.fillStyle(chip.color);
            graphics.fillCircle(radius, radius, radius);
            
            graphics.fillStyle(chip.accent);
            graphics.fillCircle(radius, radius, radius - 5);
            
            graphics.fillStyle(0xffd700);
            graphics.fillCircle(radius, radius, 6);
            
            const valueText = this.add.text(radius, radius, chip.value.toString(), {
                fontSize: chip.value >= 100 ? '10px' : '12px',
                fill: '#ffffff',
                fontFamily: 'Arial Black',
                stroke: '#000000',
                strokeThickness: 1
            }).setOrigin(0.5);
            
            graphics.generateTexture(`chip-${chip.value}`, radius * 2, radius * 2);
            valueText.destroy();
            graphics.destroy();
        });
    }

    createTableTexture() {
        const graphics = this.add.graphics();
        const width = 1200;
        const height = 800;
        
        graphics.fillGradientStyle(0x8B4513, 0xA0522D, 0x654321, 0x8B4513);
        graphics.fillEllipse(width/2, height/2, width, height);
        
        graphics.fillGradientStyle(0x0f5132, 0x1a7431, 0x0d4529, 0x185f30);
        graphics.fillEllipse(width/2, height/2, width-100, height-100);
        
        graphics.lineStyle(4, 0xffd700, 0.6);
        graphics.strokeEllipse(width/2, height/2, width-100, height-100);
        
        graphics.lineStyle(2, 0xffd700, 0.4);
        graphics.strokeCircle(width/2, height/2, 80);
        
        const positions = [
            { x: width/2, y: height - 120 },
            { x: 120, y: height/2 + 50 },
            { x: 200, y: 150 },
            { x: width/2, y: 100 },
            { x: width - 200, y: 150 },
            { x: width - 120, y: height/2 + 50 }
        ];

        positions.forEach((pos, index) => {
            graphics.lineStyle(2, 0xffd700, 0.3);
            graphics.strokeCircle(pos.x, pos.y, 60);
            
            const playerNum = this.add.text(pos.x, pos.y, (index + 1).toString(), {
                fontSize: '14px',
                fill: '#ffd700',
                fontFamily: 'Arial',
                alpha: 0.3
            }).setOrigin(0.5);
        });
        
        graphics.generateTexture('poker-table', width, height);
        graphics.destroy();
    }

    createUITextures() {
        const dealerGraphics = this.add.graphics();
        dealerGraphics.fillStyle(0xffffff);
        dealerGraphics.fillCircle(25, 25, 25);
        dealerGraphics.lineStyle(3, 0x000000);
        dealerGraphics.strokeCircle(25, 25, 22);
        
        const dealerText = this.add.text(25, 25, 'D', {
            fontSize: '24px',
            fill: '#000000',
            fontFamily: 'Arial Black'
        }).setOrigin(0.5);
        
        dealerGraphics.generateTexture('dealer-button', 50, 50);
        dealerText.destroy();
        dealerGraphics.destroy();
    }

    setupTable() {
        this.tableImage = this.add.image(400, 300, 'poker-table').setScale(0.67);
        
        this.communityCardContainer = this.add.container(400, 200);
        
        this.potContainer = this.add.container(400, 280);
        this.potBackground = this.add.graphics();
        this.potBackground.fillStyle(0x000000, 0.7);
        this.potBackground.fillRoundedRect(-80, -25, 160, 50, 25);
        this.potBackground.lineStyle(2, 0xffd700);
        this.potBackground.strokeRoundedRect(-80, -25, 160, 50, 25);
        this.potContainer.add(this.potBackground);
        
        this.potText = this.add.text(0, 0, 'POT: $0', {
            fontSize: '20px',
            fill: '#ffd700',
            fontFamily: 'Orbitron',
            fontWeight: 'bold'
        }).setOrigin(0.5);
        this.potContainer.add(this.potText);
    }

    setupUI() {
        this.playerPositions = [
            { x: 400, y: 520, name: 'bottom' },
            { x: 100, y: 350, name: 'left' },
            { x: 180, y: 150, name: 'topleft' },
            { x: 400, y: 120, name: 'top' },
            { x: 620, y: 150, name: 'topright' },
            { x: 700, y: 350, name: 'right' }
        ];
    }

    setupAnimations() {
        this.anims.create({
            key: 'card-deal',
            frames: [{ key: 'card-back', frame: 0 }],
            frameRate: 1,
            repeat: 0
        });
    }

    initializeUIElements() {
        // Get UI elements
        this.playerNameInput = document.getElementById('player-name');
        this.readyBtn = document.getElementById('ready-btn');
        this.chatMessages = document.getElementById('chat-messages');
        this.chatInput = document.getElementById('chat-input');
        this.chatSendBtn = document.getElementById('chat-send');
        this.connectionText = document.getElementById('connection-text');
        this.statusIndicator = document.getElementById('status-indicator');
        this.actionBar = document.getElementById('action-bar');
        this.potAmount = document.getElementById('pot-amount');
        this.playerCount = document.getElementById('player-count');
        this.gamePhase = document.getElementById('game-phase');
        this.myChips = document.getElementById('my-chips');
        this.resultModal = document.getElementById('result-modal');

        // Debug: Check if action bar exists
        console.log('Action bar element:', this.actionBar);
        if (!this.actionBar) {
            console.error('Action bar element not found!');
            return;
        }

        this.playerNameInput.addEventListener('input', (e) => {
            this.playerName = e.target.value.trim();
            if (this.playerName && this.socket && this.socket.readyState === WebSocket.OPEN) {
                this.sendMessage({ type: 'player_join', payload: { name: this.playerName } });
            }
        });

        this.readyBtn.addEventListener('click', () => {
            if (!this.playerName) {
                this.showMessage('Please enter your name first!', 'warning');
                return;
            }
            this.isReady = !this.isReady;
            this.updateReadyButton();
            this.sendMessage({ type: 'player_ready', payload: { isReady: this.isReady } });
        });

        this.chatSendBtn.addEventListener('click', () => this.sendChatMessage());
        this.chatInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') this.sendChatMessage();
        });

        // Action buttons
        const foldBtn = document.getElementById('fold-btn');
        const callBtn = document.getElementById('call-btn');
        const raiseBtn = document.getElementById('raise-btn');

        console.log('Action buttons:', { foldBtn, callBtn, raiseBtn });

        if (foldBtn) {
            foldBtn.addEventListener('click', () => {
                console.log('Fold button clicked');
                this.sendPlayerAction('fold');
                this.playFeedback('fold');
            });
        }

        if (callBtn) {
            callBtn.addEventListener('click', () => {
                const action = callBtn.textContent.includes('Check') ? 'check' : 'call';
                console.log('Call/Check button clicked:', action);
                this.sendPlayerAction(action);
                this.playFeedback('call');
            });
        }

        if (raiseBtn) {
            raiseBtn.addEventListener('click', () => {
                console.log('Raise button clicked');
                this.handleRaiseAction();
            });
        }
    }

    connectToServer() {
        this.updateConnectionStatus('connecting', 'Connecting...');
        this.socket = new WebSocket('ws://localhost:8080/ws');
        
        this.socket.onopen = () => {
            this.updateConnectionStatus('connected', 'Connected');
            this.reconnectAttempts = 0;
            
            if (this.playerName) {
                this.sendMessage({ type: 'player_join', payload: { name: this.playerName } });
            }
            
            // Debug message
            console.log('WebSocket connected successfully');
        };

        this.socket.onclose = () => {
            this.updateConnectionStatus('disconnected', 'Disconnected');
            this.scheduleReconnect();
        };

        this.socket.onerror = () => {
            this.updateConnectionStatus('disconnected', 'Connection Error');
        };

        this.socket.onmessage = (event) => {
            try {
                const msg = JSON.parse(event.data);
                this.handleMessage(msg);
            } catch (error) {
                console.error('Error parsing message:', error);
            }
        };
    }

    // Add debug message listener
    setupDebugListener() {
        window.addEventListener('message', (event) => {
            if (event.data.type === 'showActionButtons') {
                console.log('Debug: Force showing action buttons');
                this.showActionButtons();
                // Send debug info back
                window.parent.postMessage({
                    type: 'debug',
                    message: `Action bar display set to: ${this.actionBar.style.display}`
                }, '*');
            }
        });
    }

    updateConnectionStatus(status, text) {
        this.statusIndicator.className = `status-indicator status-${status}`;
        this.connectionText.textContent = text;
    }

    handleMessage(msg) {
        console.log('Received message:', msg);
        switch (msg.type) {
            case 'player_id':
                this.myId = msg.payload.id;
                console.log('My player ID set to:', this.myId);
                break;
            case 'game_state':
                this.gameState = msg.payload;
                console.log('Game state updated:', this.gameState);
                this.updateGameState(this.gameState);
                break;
        }
    }

    updateGameState(state) {
        this.potAmount.textContent = `$${state.pot || 0}`;
        this.playerCount.textContent = state.players ? Object.keys(state.players).length : 0;
        this.gamePhase.textContent = this.formatGamePhase(state.gamePhase || 'waiting');
        
        if (state.players && state.players[this.myId]) {
            this.myChips.textContent = `$${state.players[this.myId].chips}`;
        }

        this.potText.setText(`POT: $${state.pot || 0}`);

        if (state.playerReady) {
            this.isReady = state.playerReady[this.myId] || false;
            this.updateReadyButton();
        }

        this.readyBtn.style.display = state.gameStarted ? 'none' : 'block';

        this.updateCommunityCards(state.communityCards || []);
        this.updatePlayers(state);
        this.updateActionButtons(state);
        this.updateChatMessages(state.chatMessages || []);

        if (state.winningHandDesc && state.gamePhase === 'showdown') {
            this.showGameResult(state.winningHandDesc);
        }
    }

    updateCommunityCards(cards) {
        this.communityCardObjects.forEach(card => card.destroy());
        this.communityCardObjects = [];

        if (cards.length === 0) return;

        const startX = -(cards.length - 1) * 50;
        cards.forEach((card, index) => {
            const cardImage = this.add.image(startX + index * 100, 0, `card-${card.rank}-${card.suit}`);
            cardImage.setScale(0.8);
            cardImage.setTint(0xffffff);
            
            this.communityCardContainer.add(cardImage);
            this.communityCardObjects.push(cardImage);
            
            cardImage.setScale(0);
            this.tweens.add({
                targets: cardImage,
                scale: 0.8,
                duration: 400,
                ease: 'Back.easeOut',
                delay: index * 100
            });
        });
    }

    updatePlayers(state) {
        if (!state.players) return;

        const allPlayerIds = Object.keys(state.players);
        const otherPlayerIds = allPlayerIds.filter(id => id !== this.myId);
        
        Object.values(this.playerObjects).forEach(obj => obj.destroy());
        this.playerObjects = {};

        allPlayerIds.forEach((playerId, index) => {
            const player = state.players[playerId];
            if (!player.isConnected) return;

            const isMe = playerId === this.myId;
            const position = this.playerPositions[isMe ? 0 : (otherPlayerIds.indexOf(playerId) + 1) % 6];
            
            if (position) {
                this.renderPlayer(player, position, state, isMe);
            }
        });

        this.updateDealerButton(state);
    }

    renderPlayer(player, position, state, isMe) {
        const container = this.add.container(position.x, position.y);
        
        const bg = this.add.graphics();
        const isCurrentTurn = state.gameStarted && state.playerOrder && 
                             state.playerOrder[state.currentTurnIndex] === player.id;
        
        const bgColor = isCurrentTurn ? 0xffd700 : (isMe ? 0x4a90e2 : 0x2c3e50);
        const bgAlpha = isCurrentTurn ? 0.8 : 0.6;
        
        bg.fillStyle(bgColor, bgAlpha);
        bg.fillRoundedRect(-80, -60, 160, 120, 10);
        bg.lineStyle(2, 0xffffff, 0.8);
        bg.strokeRoundedRect(-80, -60, 160, 120, 10);
        container.add(bg);

        const nameText = this.add.text(0, -40, player.name || 'Player', {
            fontSize: '14px',
            fill: '#ffffff',
            fontFamily: 'Roboto',
            fontWeight: 'bold'
        }).setOrigin(0.5);
        container.add(nameText);

        const chipsText = this.add.text(0, -20, `$${player.chips}`, {
            fontSize: '16px',
            fill: '#ffd700',
            fontFamily: 'Orbitron',
            fontWeight: 'bold'
        }).setOrigin(0.5);
        container.add(chipsText);

        if (player.bet > 0) {
            const betText = this.add.text(0, 0, `Bet: $${player.bet}`, {
                fontSize: '12px',
                fill: '#ffffff',
                fontFamily: 'Roboto'
            }).setOrigin(0.5);
            container.add(betText);
        }

        const statusY = 20;
        if (player.isAllIn) {
            const allInText = this.add.text(0, statusY, 'ALL-IN', {
                fontSize: '10px',
                fill: '#ff6b6b',
                fontFamily: 'Roboto',
                fontWeight: 'bold'
            }).setOrigin(0.5);
            container.add(allInText);
        } else if (!player.isInHand && state.gameStarted) {
            const foldText = this.add.text(0, statusY, 'FOLDED', {
                fontSize: '10px',
                fill: '#95a5a6',
                fontFamily: 'Roboto',
                fontWeight: 'bold'
            }).setOrigin(0.5);
            container.add(foldText);
        }

        if (player.hand && player.hand.length > 0) {
            player.hand.forEach((card, cardIndex) => {
                const showCard = isMe || state.gamePhase === 'showdown';
                const cardKey = showCard ? `card-${card.rank}-${card.suit}` : 'card-back';
                
                const cardImage = this.add.image((cardIndex - 0.5) * 45, -80, cardKey);
                cardImage.setScale(0.4);
                
                if (!player.isInHand) {
                    cardImage.setTint(0x666666);
                }
                
                container.add(cardImage);
            });
        }

        this.playerObjects[player.id] = container;
        
        container.setAlpha(0);
        this.tweens.add({
            targets: container,
            alpha: 1,
            duration: 500,
            ease: 'Power2'
        });
    }

    updateDealerButton(state) {
        if (this.dealerChip) {
            this.dealerChip.destroy();
            this.dealerChip = null;
        }

        if (state.gameStarted && state.playerOrder && state.dealerIndex >= 0) {
            const dealerId = state.playerOrder[state.dealerIndex];
            const playerObj = this.playerObjects[dealerId];
            
            if (playerObj) {
                this.dealerChip = this.add.image(playerObj.x + 60, playerObj.y - 40, 'dealer-button');
                this.dealerChip.setScale(0.6);
                
                this.tweens.add({
                    targets: this.dealerChip,
                    rotation: Math.PI * 2,
                    duration: 2000,
                    ease: 'Power1',
                    repeat: -1
                });
            }
        }
    }

    updateActionButtons(state) {
        console.log('updateActionButtons called:', {
            gameStarted: state.gameStarted,
            playerOrder: state.playerOrder,
            currentTurnIndex: state.currentTurnIndex,
            myId: this.myId,
            players: state.players ? Object.keys(state.players) : []
        });

        if (!state.gameStarted || !state.playerOrder || !this.myId) {
            console.log('Hiding action bar - game not started or missing data');
            this.actionBar.style.display = 'none';
            return;
        }

        const currentPlayer = state.playerOrder[state.currentTurnIndex];
        const myTurn = currentPlayer === this.myId;
        
        console.log('Turn check:', {
            currentPlayer,
            myTurn,
            currentTurnIndex: state.currentTurnIndex,
            playerOrder: state.playerOrder
        });

        this.actionBar.style.display = myTurn ? 'flex' : 'none';

        if (myTurn && state.players[this.myId]) {
            const me = state.players[this.myId];
            const callBtn = document.getElementById('call-btn');
            
            console.log('My player data:', me);
            
            if (me.bet >= (state.lastBet || 0)) {
                callBtn.innerHTML = '<i class="fas fa-check"></i> Check';
            } else {
                const callAmount = (state.lastBet || 0) - me.bet;
                callBtn.innerHTML = `<i class="fas fa-phone"></i> Call $${callAmount}`;
            }
            
            console.log('Action bar should be visible now');
        }
    }

    updateChatMessages(messages) {
        this.chatMessages.innerHTML = '';
        
        messages.slice(-8).forEach(msg => {
            const messageDiv = document.createElement('div');
            messageDiv.className = `chat-message ${msg.playerId === 'system' ? 'system' : ''}`;
            
            if (msg.playerId === 'system') {
                messageDiv.innerHTML = `<i class="fas fa-info-circle"></i> ${msg.message}`;
            } else {
                const playerName = this.gameState.players && this.gameState.players[msg.playerId] ? 
                                 this.gameState.players[msg.playerId].name : 'Player';
                messageDiv.innerHTML = `<strong>${playerName}:</strong> ${msg.message}`;
            }
            
            this.chatMessages.appendChild(messageDiv);
        });
        
        this.chatMessages.scrollTop = this.chatMessages.scrollHeight;
    }

    sendChatMessage() {
        const message = this.chatInput.value.trim();
        if (message) {
            this.sendMessage({ type: 'chat_message', payload: { message } });
            this.chatInput.value = '';
        }
    }

    handleRaiseAction() {
        if (!this.gameState.players || !this.gameState.players[this.myId]) return;
        
        const me = this.gameState.players[this.myId];
        const minRaise = this.gameState.lastBet + (this.gameState.minRaise || 20);
        const maxRaise = me.chips + me.bet;
        
        this.createRaiseDialog(minRaise, maxRaise);
    }

    createRaiseDialog(minRaise, maxRaise) {
        const overlay = document.createElement('div');
        overlay.style.cssText = `
            position: fixed; top: 0; left: 0; right: 0; bottom: 0;
            background: rgba(0,0,0,0.8); z-index: 3000;
            display: flex; align-items: center; justify-content: center;
        `;
        
        const dialog = document.createElement('div');
        dialog.style.cssText = `
            background: linear-gradient(145deg, rgba(0,0,0,0.95), rgba(20,40,60,0.95));
            border: 2px solid #ffd700; border-radius: 15px; padding: 30px;
            backdrop-filter: blur(20px); text-align: center; min-width: 300px;
        `;
        
        dialog.innerHTML = `
            <h3 style="color: #ffd700; margin-bottom: 20px; font-family: Orbitron;">Raise Amount</h3>
            <div style="margin-bottom: 20px;">
                <label style="color: #ffffff; display: block; margin-bottom: 10px;">
                    Amount (Min: $${minRaise}, Max: $${maxRaise})
                </label>
                <input type="number" id="raise-amount" min="${minRaise}" max="${maxRaise}" value="${minRaise}"
                       style="width: 100%; padding: 10px; border: 1px solid #ffd700; border-radius: 5px;
                              background: rgba(0,0,0,0.5); color: #ffffff; font-size: 16px;">
            </div>
            <div style="display: flex; gap: 15px; justify-content: center;">
                <button id="raise-confirm" style="padding: 10px 20px; border: none; border-radius: 8px;
                        background: linear-gradient(145deg, #4488ff, #3366cc); color: white; cursor: pointer;">
                    Raise
                </button>
                <button id="raise-cancel" style="padding: 10px 20px; border: none; border-radius: 8px;
                        background: linear-gradient(145deg, #666, #444); color: white; cursor: pointer;">
                    Cancel
                </button>
            </div>
        `;
        
        overlay.appendChild(dialog);
        document.body.appendChild(overlay);
        
        const amountInput = dialog.querySelector('#raise-amount');
        const confirmBtn = dialog.querySelector('#raise-confirm');
        const cancelBtn = dialog.querySelector('#raise-cancel');
        
        confirmBtn.addEventListener('click', () => {
            const amount = parseInt(amountInput.value);
            if (amount >= minRaise && amount <= maxRaise) {
                this.sendPlayerAction('raise', { amount });
                this.playFeedback('raise');
                document.body.removeChild(overlay);
            } else {
                this.showMessage('Invalid raise amount!', 'error');
            }
        });
        
        cancelBtn.addEventListener('click', () => {
            document.body.removeChild(overlay);
        });
        
        amountInput.focus();
        amountInput.select();
    }

    showGameResult(result) {
        const modal = this.resultModal;
        const title = document.getElementById('result-title');
        const description = document.getElementById('result-description');
        
        title.textContent = 'Hand Complete';
        description.textContent = result;
        
        modal.classList.add('show');
        
        setTimeout(() => {
            modal.classList.remove('show');
        }, 4000);
    }

    updateReadyButton() {
        const btn = this.readyBtn;
        if (this.isReady) {
            btn.innerHTML = '<i class="fas fa-times"></i> Cancel';
            btn.classList.add('ready');
        } else {
            btn.innerHTML = '<i class="fas fa-play"></i> Ready to Play';
            btn.classList.remove('ready');
        }
    }

    sendMessage(message) {
        if (this.socket && this.socket.readyState === WebSocket.OPEN) {
            this.socket.send(JSON.stringify(message));
        }
    }

    sendPlayerAction(action, payload = {}) {
        console.log('Sending player action:', action, payload);
        this.sendMessage({ type: 'player_action', payload: { action, ...payload } });
        this.actionBar.style.display = 'none';
    }

    // Debug function to manually show action buttons for testing
    showActionButtons() {
        console.log('Manually showing action buttons for testing');
        if (this.actionBar) {
            this.actionBar.style.display = 'flex';
        }
    }

    playFeedback(type) {
        this.cameras.main.flash(100, 255, 255, 255, false);
    }

    showMessage(text, type = 'info') {
        const toast = document.createElement('div');
        toast.style.cssText = `
            position: fixed; top: 20px; right: 20px; z-index: 4000;
            background: ${type === 'error' ? '#ff4444' : type === 'warning' ? '#ffaa00' : '#4488ff'};
            color: white; padding: 15px 20px; border-radius: 8px;
            font-family: Roboto; font-size: 14px; font-weight: 500;
            box-shadow: 0 4px 15px rgba(0,0,0,0.3);
        `;
        toast.textContent = text;
        
        document.body.appendChild(toast);
        
        setTimeout(() => {
            document.body.removeChild(toast);
        }, 3000);
    }

    formatGamePhase(phase) {
        const phases = {
            'waiting': 'Waiting for Players',
            'pre-flop': 'Pre-Flop',
            'flop': 'Flop',
            'turn': 'Turn',
            'river': 'River',
            'showdown': 'Showdown'
        };
        return phases[phase] || phase;
    }

    scheduleReconnect() {
        if (this.reconnectAttempts < 10) {
            const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
            this.reconnectAttempts++;
            setTimeout(() => this.connectToServer(), delay);
        } else {
            this.updateConnectionStatus('disconnected', 'Connection Failed');
        }
    }
}

const config = {
    type: Phaser.AUTO,
    width: 800,
    height: 600,
    parent: 'game-container',
    backgroundColor: 'transparent',
    scene: GameScene,
    physics: {
        default: 'arcade',
        arcade: {
            gravity: { y: 0 }
        }
    }
};

new Phaser.Game(config);