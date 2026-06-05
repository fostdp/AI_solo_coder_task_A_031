class WebSocketClient {
    constructor(url) {
        this.url = url;
        this.ws = null;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 10;
        this.reconnectInterval = 3000;
        this.heartbeatInterval = null;
        this.callbacks = {};
        this.isConnected = false;
    }

    connect() {
        try {
            this.ws = new WebSocket(this.url);
            
            this.ws.onopen = () => this.onOpen();
            this.ws.onmessage = (e) => this.onMessage(e);
            this.ws.onerror = (e) => this.onError(e);
            this.ws.onclose = (e) => this.onClose(e);
        } catch (error) {
            console.error('WebSocket connection error:', error);
            this.scheduleReconnect();
        }
    }

    onOpen() {
        console.log('WebSocket connected');
        this.isConnected = true;
        this.reconnectAttempts = 0;
        this.updateConnectionStatus(true);
        this.startHeartbeat();
        this.emit('connected');
    }

    onMessage(event) {
        try {
            const message = JSON.parse(event.data);
            this.handleMessage(message);
        } catch (error) {
            console.error('Failed to parse WebSocket message:', error);
        }
    }

    onError(event) {
        console.error('WebSocket error:', event);
        this.emit('error', event);
    }

    onClose(event) {
        console.log('WebSocket disconnected:', event.code, event.reason);
        this.isConnected = false;
        this.updateConnectionStatus(false);
        this.stopHeartbeat();
        this.emit('disconnected');
        this.scheduleReconnect();
    }

    handleMessage(message) {
        switch (message.type) {
            case 'heartbeat':
                this.sendHeartbeat();
                break;
            case 'sensor_data':
                this.emit('sensor_data', message.data);
                break;
            case 'control_command':
                this.emit('control_command', message.data);
                break;
            case 'alert':
                this.emit('alert', message.data);
                break;
            case 'kpi_update':
                this.emit('kpi_update', message.data);
                break;
            case 'system_status':
                this.emit('system_status', message.data);
                break;
            default:
                console.log('Unknown message type:', message.type);
        }
    }

    send(message) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify(message));
        } else {
            console.warn('WebSocket not connected, cannot send message');
        }
    }

    startHeartbeat() {
        this.heartbeatInterval = setInterval(() => {
            if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                this.send({ type: 'heartbeat', timestamp: Date.now() });
            }
        }, 30000);
    }

    stopHeartbeat() {
        if (this.heartbeatInterval) {
            clearInterval(this.heartbeatInterval);
            this.heartbeatInterval = null;
        }
    }

    sendHeartbeat() {
        this.send({ type: 'heartbeat_ack', timestamp: Date.now() });
    }

    scheduleReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('Max reconnection attempts reached');
            this.updateConnectionStatus(false, '重连失败');
            return;
        }

        this.reconnectAttempts++;
        const delay = this.reconnectInterval * Math.pow(1.5, this.reconnectAttempts - 1);
        
        console.log(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
        this.updateConnectionStatus(false, `重连中 (${this.reconnectAttempts})`);
        
        setTimeout(() => {
            this.connect();
        }, delay);
    }

    updateConnectionStatus(connected, text = '') {
        const statusEl = document.getElementById('connectionStatus');
        const iconEl = document.getElementById('connectionIcon');
        
        if (statusEl) {
            statusEl.textContent = text || (connected ? '已连接' : '未连接');
            statusEl.className = 'connection-status ' + (connected ? 'connected' : 'disconnected');
        }
        
        if (iconEl) {
            iconEl.className = 'fas fa-wifi connection-icon ' + (connected ? 'connected' : 'disconnected');
        }
    }

    on(event, callback) {
        if (!this.callbacks[event]) {
            this.callbacks[event] = [];
        }
        this.callbacks[event].push(callback);
    }

    off(event, callback) {
        if (this.callbacks[event]) {
            this.callbacks[event] = this.callbacks[event].filter(cb => cb !== callback);
        }
    }

    emit(event, data) {
        if (this.callbacks[event]) {
            this.callbacks[event].forEach(callback => {
                try {
                    callback(data);
                } catch (error) {
                    console.error(`Error in callback for event '${event}':`, error);
                }
            });
        }
    }

    disconnect() {
        this.stopHeartbeat();
        if (this.ws) {
            this.ws.close();
        }
    }
}
