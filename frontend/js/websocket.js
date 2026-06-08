const WebSocketClient = {
    ws: null,
    reconnectAttempts: 0,
    maxReconnectAttempts: 10,
    reconnectInterval: 5000,
    isConnected: false,

    init() {
        this.connect();
    },

    connect() {
        try {
            this.ws = new WebSocket(CONFIG.WS_URL);

            this.ws.onopen = () => {
                console.log('WebSocket已连接');
                this.isConnected = true;
                this.reconnectAttempts = 0;
                this.updateConnectionStatus(true);
            };

            this.ws.onmessage = (event) => {
                try {
                    const message = JSON.parse(event.data);
                    this.handleMessage(message);
                } catch (e) {
                    console.error('解析WebSocket消息失败:', e);
                }
            };

            this.ws.onclose = (event) => {
                console.log('WebSocket连接关闭:', event.code, event.reason);
                this.isConnected = false;
                this.updateConnectionStatus(false);
                this.attemptReconnect();
            };

            this.ws.onerror = (error) => {
                console.error('WebSocket错误:', error);
                this.isConnected = false;
                this.updateConnectionStatus(false);
            };
        } catch (e) {
            console.error('创建WebSocket连接失败:', e);
            this.attemptReconnect();
        }
    },

    attemptReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('达到最大重连次数，停止重连');
            return;
        }

        this.reconnectAttempts++;
        const delay = Math.min(this.reconnectInterval * Math.pow(1.5, this.reconnectAttempts - 1), 30000);
        
        console.log(`尝试重连 (${this.reconnectAttempts}/${this.maxReconnectAttempts})，${delay/1000}秒后...`);
        
        setTimeout(() => {
            this.connect();
        }, delay);
    },

    handleMessage(message) {
        if (!message.type) return;

        switch (message.type) {
            case 'sensor_data':
                this.handleSensorData(message.data);
                break;
            case 'sensor_update':
                this.handleSensorUpdate(message.data);
                break;
            case 'aeration_status':
                this.handleAerationStatus(message.data);
                break;
            case 'carbon_status':
                this.handleCarbonStatus(message.data);
                break;
            case 'alarm':
                this.handleAlarm(message.data);
                break;
            case 'alarm_update':
                this.handleAlarmUpdate(message.data);
                break;
            case 'metrics':
                this.handleMetrics(message.data);
                break;
            case 'control_command':
                this.handleControlCommand(message.data);
                break;
            case 'ping':
                this.sendPong();
                break;
            default:
                console.log('未知消息类型:', message.type);
        }
    },

    handleSensorData(data) {
        if (BioreactorProfile) {
            BioreactorProfile.updateSensor(data);
        }
    },

    handleSensorUpdate(data) {
        if (data && data.sensors) {
            data.sensors.forEach(sensor => {
                if (BioreactorProfile) {
                    BioreactorProfile.updateSensor(sensor);
                }
            });
        }
    },

    handleAerationStatus(data) {
        if (ControlPanel) {
            if (data.section !== undefined) {
                ControlPanel.updateAerationSection(data);
            } else if (data.sections) {
                ControlPanel.aerationStatus = data.sections;
                ControlPanel.renderAerationStatus();
            }
        }
    },

    handleCarbonStatus(data) {
        if (ControlPanel) {
            ControlPanel.updateCarbonStatus(data);
        }
    },

    handleAlarm(data) {
        if (AlarmManager) {
            AlarmManager.addAlarm(data);
        }
    },

    handleAlarmUpdate(data) {
        if (AlarmManager) {
            if (data.id && data.acknowledged) {
                AlarmManager.updateAlarm(data.id, { 
                    acknowledged: true, 
                    acknowledged_by: data.acknowledged_by,
                    acknowledged_at: data.acknowledged_at 
                });
            } else if (data.id && data.cleared) {
                AlarmManager.removeAlarm(data.id);
            }
        }
    },

    handleMetrics(data) {
        this.updateMetricsDisplay(data);
    },

    handleControlCommand(data) {
        console.log('收到控制指令:', data);
        if (ControlPanel) {
            ControlPanel.showNotification(
                `控制指令已下发: ${data.command || data.type}`, 
                data.success ? 'success' : 'error'
            );
        }
    },

    updateMetricsDisplay(data) {
        if (data.power_consumption !== undefined) {
            const el = document.getElementById('metric-power');
            if (el) el.textContent = data.power_consumption.toFixed(2);
        }
        if (data.carbon_usage !== undefined) {
            const el = document.getElementById('metric-carbon');
            if (el) el.textContent = data.carbon_usage.toFixed(2);
        }
        if (data.tn_removal_rate !== undefined) {
            const el = document.getElementById('metric-tn');
            if (el) el.textContent = data.tn_removal_rate.toFixed(1);
        }
        if (data.tp_removal_rate !== undefined) {
            const el = document.getElementById('metric-tp');
            if (el) el.textContent = data.tp_removal_rate.toFixed(1);
        }
    },

    updateConnectionStatus(connected) {
        const statusIndicator = document.getElementById('ws-status');
        if (statusIndicator) {
            statusIndicator.textContent = connected ? '● 已连接' : '○ 断开';
            statusIndicator.className = connected ? 'ws-connected' : 'ws-disconnected';
        }
    },

    sendPong() {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify({ type: 'pong' }));
        }
    },

    send(message) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify(message));
            return true;
        }
        console.warn('WebSocket未连接，无法发送消息');
        return false;
    },

    close() {
        if (this.ws) {
            this.ws.close();
        }
    }
};

window.addEventListener('beforeunload', () => {
    WebSocketClient.close();
});
