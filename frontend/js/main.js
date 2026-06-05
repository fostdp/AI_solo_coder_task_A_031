class SewagePlantApp {
    constructor() {
        this.processLayout = null;
        this.biologicalProfile = null;
        this.sensorTrend = null;
        this.websocket = null;
        this.kpiDashboard = null;
        this.activeTab = 'layout';
        this.alertHistory = [];
        this.sensorData = {};
        this.init();
    }

    init() {
        this.setupTabs();
        this.setupModal();
        this.setupWebSocket();
        this.setupCanvas();
        this.setupKPIDashboard();
        this.setupPeriodicUpdates();
        this.loadInitialData();
    }

    setupTabs() {
        const tabs = document.querySelectorAll('.tab-btn');
        const panels = document.querySelectorAll('.tab-content');

        tabs.forEach(tab => {
            tab.addEventListener('click', () => {
                const tabId = tab.dataset.tab;
                
                tabs.forEach(t => t.classList.remove('active'));
                panels.forEach(p => p.classList.remove('active'));
                
                tab.classList.add('active');
                document.getElementById(tabId).classList.add('active');
                
                this.activeTab = tabId;
                this.onTabChange(tabId);
            });
        });
    }

    onTabChange(tabId) {
        if (tabId === 'trends') {
            this.loadTrendData();
        } else if (tabId === 'kpi') {
            this.loadKPIData();
        }
    }

    setupModal() {
        const modal = document.getElementById('sensorModal');
        const closeBtn = modal.querySelector('.close-btn');
        const closeModal = modal.querySelector('[data-close="modal"]');

        closeBtn.addEventListener('click', () => {
            modal.classList.remove('active');
        });

        closeModal.addEventListener('click', () => {
            modal.classList.remove('active');
        });

        modal.addEventListener('click', (e) => {
            if (e.target === modal) {
                modal.classList.remove('active');
            }
        });
    }

    setupWebSocket() {
        this.websocket = new WebSocketClient(CONFIG.WS_URL);
        
        this.websocket.on('sensor_data', (data) => {
            this.handleSensorData(data);
        });
        
        this.websocket.on('alert', (alert) => {
            this.handleAlert(alert);
        });
        
        this.websocket.on('kpi_update', (data) => {
            this.handleKPIUpdate(data);
        });
        
        this.websocket.on('control_command', (cmd) => {
            this.handleControlCommand(cmd);
        });
        
        this.websocket.on('system_status', (status) => {
            this.handleSystemStatus(status);
        });
        
        this.websocket.connect();
    }

    setupCanvas() {
        this.processLayout = new ProcessLayout('layoutCanvas');
        this.biologicalProfile = new BiologicalProfile('profileCanvas');
        this.sensorTrend = new SensorTrend('trendCanvas', {
            label: '参数趋势',
            unit: '',
            color: CONFIG.COLORS.primary
        });
    }

    setupKPIDashboard() {
        const containers = {
            energy: document.querySelector('.kpi-card:nth-child(1)'),
            carbon: document.querySelector('.kpi-card:nth-child(2)'),
            removal: document.querySelector('.kpi-card:nth-child(3)')
        };
        
        this.kpiDashboard = new KPIDashboard(containers);
    }

    setupPeriodicUpdates() {
        setInterval(() => {
            if (this.activeTab === 'trends') {
                this.loadTrendData();
            }
        }, 30000);
        
        setInterval(() => {
            this.loadSystemStatus();
        }, 5000);
    }

    async loadInitialData() {
        await this.loadSensorList();
        await this.loadSystemStatus();
        await this.loadAlerts();
        this.startSimulation();
    }

    async loadSensorList() {
        try {
            const response = await fetch(`${CONFIG.API_BASE_URL}/sensors`);
            const sensors = await response.json();
            
            sensors.forEach(sensor => {
                const section = CONFIG.PROCESS_SECTIONS.find(s => s.id === sensor.location);
                if (section) {
                    sensor.x = section.x + section.width / 2 + (Math.random() - 0.5) * section.width * 0.6;
                    sensor.y = section.y + section.height / 2 + (Math.random() - 0.5) * section.height * 0.6;
                }
            });
            
            if (this.processLayout) {
                this.processLayout.sensors = sensors;
                this.processLayout.draw();
            }
        } catch (error) {
            console.error('Failed to load sensor list:', error);
        }
    }

    async loadSystemStatus() {
        try {
            const response = await fetch(`${CONFIG.API_BASE_URL}/system/status`);
            const status = await response.json();
            this.updateSystemStatus(status);
        } catch (error) {
            console.error('Failed to load system status:', error);
        }
    }

    async loadAlerts() {
        try {
            const response = await fetch(`${CONFIG.API_BASE_URL}/alerts?limit=10`);
            const alerts = await response.json();
            
            const alertList = document.getElementById('alertList');
            if (alertList) {
                alertList.innerHTML = '';
                alerts.forEach(alert => {
                    this.addAlertToList(alert);
                });
            }
        } catch (error) {
            console.error('Failed to load alerts:', error);
        }
    }

    async loadTrendData() {
        const trendTypes = ['DO', 'NH3', 'NO3', 'PO4'];
        
        for (const type of trendTypes) {
            try {
                const response = await fetch(`${CONFIG.API_BASE_URL}/trend/${type}?hours=24`);
                const data = await response.json();
                
                const canvas = document.getElementById(`${type.toLowerCase()}TrendChart`);
                if (canvas) {
                    const ctx = canvas.getContext('2d');
                    const container = canvas.parentElement;
                    canvas.width = container.clientWidth;
                    canvas.height = 250;
                    
                    drawLineChart(ctx, canvas, data.timestamps, data.values, {
                        color: CONFIG.SENSOR_TYPES[type].color,
                        label: CONFIG.SENSOR_TYPES[type].name,
                        unit: CONFIG.SENSOR_TYPES[type].unit
                    });
                }
            } catch (error) {
                console.error(`Failed to load ${type} trend data:`, error);
            }
        }
    }

    async loadKPIData() {
        try {
            const response = await fetch(`${CONFIG.API_BASE_URL}/kpi/history?days=7`);
            const data = await response.json();
            
            const energyCanvas = document.getElementById('energyHistoryChart');
            if (energyCanvas) {
                const ctx = energyCanvas.getContext('2d');
                energyCanvas.width = energyCanvas.parentElement.clientWidth;
                energyCanvas.height = 300;
                
                drawLineChart(ctx, energyCanvas, data.timestamps, data.energy, {
                    color: '#ef4444',
                    label: '吨水电耗',
                    unit: 'kWh/吨'
                });
            }
            
            const carbonCanvas = document.getElementById('carbonHistoryChart');
            if (carbonCanvas) {
                const ctx = carbonCanvas.getContext('2d');
                carbonCanvas.width = carbonCanvas.parentElement.clientWidth;
                carbonCanvas.height = 300;
                
                drawLineChart(ctx, carbonCanvas, data.timestamps, data.carbon, {
                    color: '#f59e0b',
                    label: '碳源单耗',
                    unit: 'kg/吨'
                });
            }
            
            const removalCanvas = document.getElementById('removalHistoryChart');
            if (removalCanvas) {
                const ctx = removalCanvas.getContext('2d');
                removalCanvas.width = removalCanvas.parentElement.clientWidth;
                removalCanvas.height = 300;
                
                drawLineChart(ctx, removalCanvas, data.timestamps, data.removal, {
                    color: '#10b981',
                    label: '总氮去除率',
                    unit: '%'
                });
            }
            
            const qualityCanvas = document.getElementById('qualityGaugeChart');
            if (qualityCanvas) {
                const ctx = qualityCanvas.getContext('2d');
                qualityCanvas.width = 250;
                qualityCanvas.height = 250;
                
                drawGauge(ctx, qualityCanvas, data.quality || 85, 0, 100, 90, {
                    color: '#10b981',
                    label: '综合水质评分',
                    unit: '分'
                });
            }
            
            const removalBreakdownCanvas = document.getElementById('removalBreakdownChart');
            if (removalBreakdownCanvas) {
                const ctx = removalBreakdownCanvas.getContext('2d');
                removalBreakdownCanvas.width = 400;
                removalBreakdownCanvas.height = 300;
                
                drawDonutChart(ctx, removalBreakdownCanvas, [
                    { label: 'COD', value: data.codRemoval || 92, color: '#10b981' },
                    { label: 'NH3-N', value: data.nh3Removal || 95, color: '#3b82f6' },
                    { label: 'TN', value: data.tnRemoval || 78, color: '#8b5cf6' },
                    { label: 'TP', value: data.tpRemoval || 93, color: '#f59e0b' }
                ], {
                    title: '污染物去除率 (%)'
                });
            }
        } catch (error) {
            console.error('Failed to load KPI data:', error);
        }
    }

    handleSensorData(data) {
        this.sensorData[data.sensor_id] = data;
        
        if (this.processLayout) {
            this.processLayout.updateSensorValue(data.sensor_id, data.value, data.timestamp);
        }
        
        this.updateSensorDashboard(data);
    }

    handleAlert(alert) {
        this.alertHistory.unshift(alert);
        if (this.alertHistory.length > 50) {
            this.alertHistory.pop();
        }
        
        this.addAlertToList(alert);
        this.showAlertNotification(alert);
        
        const alertCount = document.getElementById('alertCount');
        if (alertCount) {
            const unread = this.alertHistory.filter(a => !a.acknowledged).length;
            alertCount.textContent = unread;
            alertCount.style.display = unread > 0 ? 'inline' : 'none';
        }
    }

    handleKPIUpdate(data) {
        if (this.kpiDashboard) {
            if (data.energy_per_ton !== undefined) {
                this.kpiDashboard.update('energy', data.energy_per_ton, CONFIG.KPI_TARGETS.energyPerTon);
            }
            if (data.carbon_per_ton !== undefined) {
                this.kpiDashboard.update('carbon', data.carbon_per_ton, CONFIG.KPI_TARGETS.carbonPerTon);
            }
            if (data.tn_removal_rate !== undefined) {
                this.kpiDashboard.update('removal', data.tn_removal_rate, CONFIG.KPI_TARGETS.tnRemovalRate);
            }
        }
    }

    handleControlCommand(cmd) {
        const controlStatus = document.getElementById('controlStatus');
        if (controlStatus) {
            const time = new Date(cmd.timestamp).toLocaleTimeString('zh-CN');
            controlStatus.innerHTML = `
                <div class="control-item">
                    <span class="control-time">${time}</span>
                    <span class="control-target">${cmd.target}</span>
                    <span class="control-value">${cmd.value.toFixed(2)}</span>
                </div>
            ` + controlStatus.innerHTML;
        }
    }

    handleSystemStatus(status) {
        this.updateSystemStatus(status);
    }

    updateSensorDashboard(data) {
        const type = data.sensor_id.split('-')[0];
        const container = document.getElementById(`sensor-${type.toLowerCase()}`);
        
        if (container) {
            const valueEl = container.querySelector('.sensor-value');
            const statusEl = container.querySelector('.sensor-status');
            
            if (valueEl) {
                valueEl.textContent = `${data.value.toFixed(3)} ${CONFIG.SENSOR_TYPES[type].unit}`;
            }
            
            if (statusEl) {
                const setpoint = CONFIG.SENSOR_TYPES[type].setpoint;
                const deviation = Math.abs((data.value - setpoint) / setpoint * 100);
                
                if (deviation < 10) {
                    statusEl.textContent = '正常';
                    statusEl.className = 'sensor-status status-normal';
                } else if (deviation < 20) {
                    statusEl.textContent = '警告';
                    statusEl.className = 'sensor-status status-warning';
                } else {
                    statusEl.textContent = '异常';
                    statusEl.className = 'sensor-status status-alert';
                }
            }
        }
    }

    updateSystemStatus(status) {
        const blowerStatus = document.getElementById('blowerStatus');
        if (blowerStatus && status.blowers) {
            const running = status.blowers.filter(b => b.running).length;
            blowerStatus.innerHTML = `<span class="status-dot ${running > 0 ? 'status-online' : 'status-offline'}"></span>运行 ${running}/${status.blowers.length}`;
        }
        
        const valveStatus = document.getElementById('valveStatus');
        if (valveStatus && status.valves) {
            const online = status.valves.filter(v => v.online).length;
            valveStatus.innerHTML = `<span class="status-dot ${online === status.valves.length ? 'status-online' : 'status-warning'}"></span>在线 ${online}/${status.valves.length}`;
        }
        
        const sensorOnline = document.getElementById('sensorOnline');
        if (sensorOnline && status.sensors) {
            sensorOnline.innerHTML = `<span class="status-dot ${status.sensors.online_rate > 0.9 ? 'status-online' : 'status-warning'}"></span>${(status.sensors.online_rate * 100).toFixed(0)}% 在线`;
        }
    }

    addAlertToList(alert) {
        const alertList = document.getElementById('alertList');
        if (!alertList) return;
        
        const alertItem = document.createElement('div');
        alertItem.className = `alert-item alert-${alert.level}`;
        
        const levelText = alert.level === 1 ? '一级告警' : '二级告警';
        const time = new Date(alert.timestamp).toLocaleString('zh-CN');
        
        alertItem.innerHTML = `
            <div class="alert-header">
                <span class="alert-level">${levelText}</span>
                <span class="alert-time">${time}</span>
            </div>
            <div class="alert-message">${alert.message}</div>
            ${alert.sensor_id ? `<div class="alert-sensor">传感器: ${alert.sensor_id}</div>` : ''}
        `;
        
        alertList.insertBefore(alertItem, alertList.firstChild);
        
        while (alertList.children.length > 10) {
            alertList.removeChild(alertList.lastChild);
        }
    }

    showAlertNotification(alert) {
        if (!('Notification' in window)) return;
        
        if (Notification.permission === 'granted') {
            new Notification(alert.level === 1 ? '⚠️ 一级告警' : '⚠️ 二级告警', {
                body: alert.message,
                icon: '/icon.png'
            });
        } else if (Notification.permission !== 'denied') {
            Notification.requestPermission();
        }
    }

    startSimulation() {
        setInterval(() => {
            if (!this.websocket.isConnected) {
                this.generateMockData();
            }
        }, 5000);
    }

    generateMockData() {
        const types = ['DO', 'NH3', 'NO3', 'PO4'];
        const now = new Date();
        
        for (const type of types) {
            const count = type === 'DO' ? 30 : type === 'NH3' ? 20 : type === 'NO3' ? 15 : 10;
            const numToUpdate = Math.floor(Math.random() * 3) + 1;
            
            for (let i = 0; i < numToUpdate; i++) {
                const sensorNum = Math.floor(Math.random() * count) + 1;
                const sensorId = `${type}-${String(sensorNum).padStart(3, '0')}`;
                const setpoint = CONFIG.SENSOR_TYPES[type].setpoint;
                const value = setpoint * (0.85 + Math.random() * 0.3);
                
                this.handleSensorData({
                    sensor_id: sensorId,
                    type: type,
                    value: value,
                    timestamp: now,
                    location: ['aerobic1', 'aerobic2', 'aerobic3', 'anoxic', 'effluent'][Math.floor(Math.random() * 5)]
                });
            }
        }
        
        if (Math.random() < 0.02) {
            this.handleKPIUpdate({
                energy_per_ton: 0.30 + Math.random() * 0.1,
                carbon_per_ton: 0.20 + Math.random() * 0.1,
                tn_removal_rate: 75 + Math.random() * 10
            });
        }
        
        if (Math.random() < 0.005) {
            this.handleAlert({
                level: Math.random() < 0.3 ? 1 : 2,
                message: Math.random() < 0.5 
                    ? '出水氨氮超过5mg/L，请检查处理工艺' 
                    : 'DO传感器通讯异常，请检查设备状态',
                timestamp: now,
                sensor_id: Math.random() < 0.5 ? 'DO-005' : null
            });
        }
    }
}

document.addEventListener('DOMContentLoaded', () => {
    window.app = new SewagePlantApp();
    
    window.addEventListener('resize', () => {
        if (window.app.processLayout) {
            window.app.processLayout.resize();
        }
        if (window.app.biologicalProfile) {
            window.app.biologicalProfile.resize();
        }
        if (window.app.sensorTrend) {
            window.app.sensorTrend.resize();
        }
    });
});
