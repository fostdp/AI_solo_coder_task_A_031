const AppState = {
    currentView: 'flow',
    sensors: [],
    sensorStatus: {},
    alarmCount: 0,
    kpiData: {
        energy: 0,
        carbon: 0,
        removal: 0
    },
    controlData: {
        aeration: [],
        carbon: null
    },
    selectedSensor: null
};

const API_BASE = '/api';
let ws = null;
let mainCanvas = null;
let mainCtx = null;

const stageNames = {
    'coarse_screen': '粗格栅',
    'fine_screen': '细格栅',
    'grit_chamber': '沉砂池',
    'primary_settling': '初沉池',
    'anaerobic': '厌氧池',
    'anoxic': '缺氧池',
    'aerobic': '好氧池',
    'secondary_settling': '二沉池',
    'advanced_treatment': '深度处理',
    'effluent': '出水'
};

const sensorTypeNames = {
    'DO': '溶解氧',
    'NH3': '氨氮',
    'NO3': '硝氮',
    'PO4': '磷酸盐',
    'COD': 'COD',
    'TN': '总氮',
    'TP': '总磷',
    'MLSS': 'MLSS'
};

document.addEventListener('DOMContentLoaded', function() {
    mainCanvas = document.getElementById('main-canvas');
    mainCtx = mainCanvas.getContext('2d');

    initTabs();
    initModal();
    initCanvasClick();

    loadSensorConfigs();
    loadAllData();
    connectWebSocket();
    drawCanvas();

    setInterval(loadAllData, 5000);
});

function initTabs() {
    const tabBtns = document.querySelectorAll('.tab-btn');
    tabBtns.forEach(btn => {
        btn.addEventListener('click', function() {
            tabBtns.forEach(b => b.classList.remove('active'));
            this.classList.add('active');
            AppState.currentView = this.dataset.view;
            drawCanvas();
        });
    });
}

function initModal() {
    const modal = document.getElementById('sensor-modal');
    const closeBtn = document.getElementById('close-modal');

    closeBtn.addEventListener('click', function() {
        modal.classList.remove('active');
        AppState.selectedSensor = null;
    });

    modal.addEventListener('click', function(e) {
        if (e.target === modal) {
            modal.classList.remove('active');
            AppState.selectedSensor = null;
        }
    });
}

function initCanvasClick() {
    mainCanvas.addEventListener('click', function(e) {
        const rect = mainCanvas.getBoundingClientRect();
        const x = e.clientX - rect.left;
        const y = e.clientY - rect.top;
        const scaleX = mainCanvas.width / rect.width;
        const scaleY = mainCanvas.height / rect.height;
        const canvasX = x * scaleX;
        const canvasY = y * scaleY;

        for (const sensor of AppState.sensors) {
            const status = AppState.sensorStatus[sensor.id];
            const sensorX = sensor.x;
            const sensorY = sensor.y;
            const distance = Math.sqrt((canvasX - sensorX) ** 2 + (canvasY - sensorY) ** 2);

            if (distance < 12) {
                showSensorDetail(sensor, status);
                return;
            }
        }
    });
}

async function loadSensorConfigs() {
    try {
        const response = await fetch(`${API_BASE}/sensors`);
        const data = await response.json();
        if (data.code === 0) {
            AppState.sensors = data.data;
            drawCanvas();
        }
    } catch (error) {
        console.error('Failed to load sensor configs:', error);
    }
}

async function loadAllData() {
    Promise.all([
        loadSensorStatus(),
        loadKPIs(),
        loadAerationStatus(),
        loadCarbonStatus(),
        loadAlarms(),
        loadKPITrends()
    ]).then(() => {
        updateUI();
        drawCanvas();
    });
}

async function loadSensorStatus() {
    try {
        const response = await fetch(`${API_BASE}/sensors/status`);
        const data = await response.json();
        if (data.code === 0) {
            const statusMap = {};
            let onlineCount = 0;
            data.data.forEach(s => {
                statusMap[s.id] = s;
                if (s.online) onlineCount++;
            });
            AppState.sensorStatus = statusMap;
            document.getElementById('online-count').textContent = `${onlineCount}/${AppState.sensors.length}`;
        }
    } catch (error) {
        console.error('Failed to load sensor status:', error);
    }
}

async function loadKPIs() {
    try {
        const response = await fetch(`${API_BASE}/kpi`);
        const data = await response.json();
        if (data.code === 0) {
            AppState.kpiData = {
                energy: data.data.energy_consumption,
                carbon: data.data.carbon_consumption,
                removal: data.data.removal_rate
            };
        }
    } catch (error) {
        console.error('Failed to load KPIs:', error);
    }
}

async function loadKPITrends() {
    try {
        const [energyTrend, carbonTrend, removalTrend] = await Promise.all([
            fetch(`${API_BASE}/kpi/energy_consumption/trend?days=7`).then(r => r.json()),
            fetch(`${API_BASE}/kpi/carbon_consumption/trend?days=7`).then(r => r.json()),
            fetch(`${API_BASE}/kpi/removal_rate/trend?days=7`).then(r => r.json())
        ]);

        if (energyTrend.code === 0) {
            drawMiniChart('kpi-energy-chart', energyTrend.data, '#2196F3', 'kWh/m³');
        }
        if (carbonTrend.code === 0) {
            drawMiniChart('kpi-carbon-chart', carbonTrend.data, '#9C27B0', 'kg/m³');
        }
        if (removalTrend.code === 0) {
            drawMiniChart('kpi-removal-chart', removalTrend.data, '#4CAF50', '%');
        }
    } catch (error) {
        console.error('Failed to load KPI trends:', error);
    }
}

async function loadAerationStatus() {
    try {
        const response = await fetch(`${API_BASE}/control/aeration`);
        const data = await response.json();
        if (data.code === 0) {
            AppState.controlData.aeration = data.data;
        }
    } catch (error) {
        console.error('Failed to load aeration status:', error);
    }
}

async function loadCarbonStatus() {
    try {
        const response = await fetch(`${API_BASE}/control/carbon`);
        const data = await response.json();
        if (data.code === 0) {
            AppState.controlData.carbon = data.data;
        }
    } catch (error) {
        console.error('Failed to load carbon status:', error);
    }
}

async function loadAlarms() {
    try {
        const response = await fetch(`${API_BASE}/alarms?active=true`);
        const data = await response.json();
        if (data.code === 0) {
            AppState.alarmCount = data.data.length;
            updateAlarmList(data.data);
        }
    } catch (error) {
        console.error('Failed to load alarms:', error);
    }
}

function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    ws = new WebSocket(wsUrl);

    ws.onopen = function() {
        console.log('WebSocket connected');
    };

    ws.onmessage = function(event) {
        try {
            const data = JSON.parse(event.data);
            handleWebSocketMessage(data);
        } catch (error) {
            console.error('Failed to parse WebSocket message:', error);
        }
    };

    ws.onclose = function() {
        console.log('WebSocket disconnected, reconnecting...');
        setTimeout(connectWebSocket, 3000);
    };

    ws.onerror = function(error) {
        console.error('WebSocket error:', error);
    };
}

function handleWebSocketMessage(data) {
    switch (data.type) {
        case 'sensor_update':
            AppState.sensorStatus[data.sensor.id] = data.sensor;
            drawCanvas();
            updateUI();
            break;
        case 'alarm':
            AppState.alarmCount++;
            loadAlarms();
            break;
        case 'kpi_update':
            AppState.kpiData = data.kpi;
            updateUI();
            break;
        case 'control_update':
            AppState.controlData.aeration = data.aeration;
            AppState.controlData.carbon = data.carbon;
            updateUI();
            break;
        case 'heartbeat':
            break;
    }
}

function updateUI() {
    document.getElementById('kpi-energy').textContent = AppState.kpiData.energy.toFixed(3);
    document.getElementById('kpi-carbon').textContent = AppState.kpiData.carbon.toFixed(4);
    document.getElementById('kpi-removal').textContent = AppState.kpiData.removal.toFixed(1);

    document.getElementById('alarm-count').textContent = AppState.alarmCount;

    if (AppState.controlData.aeration.length >= 3) {
        document.getElementById('aeration-a').textContent = AppState.controlData.aeration[0].aeration_rate.toFixed(1) + '%';
        document.getElementById('aeration-b').textContent = AppState.controlData.aeration[1].aeration_rate.toFixed(1) + '%';
        document.getElementById('aeration-c').textContent = AppState.controlData.aeration[2].aeration_rate.toFixed(1) + '%';
    }

    if (AppState.controlData.carbon) {
        document.getElementById('carbon-dosage').textContent = AppState.controlData.carbon.dosage_rate.toFixed(1) + ' L/h';
        document.getElementById('anoxic-no3').textContent = AppState.controlData.carbon.no3.toFixed(2) + ' mg/L';
        document.getElementById('tn-removal').textContent = AppState.controlData.carbon.tn_removal.toFixed(1) + '%';
    }

    const codSensor = AppState.sensors.find(s => s.id === 'COD-IN');
    if (codSensor && AppState.sensorStatus['COD-IN']) {
        document.getElementById('influent-cod').textContent = AppState.sensorStatus['COD-IN'].value.toFixed(1) + ' mg/L';
    }

    const nh3EffSensor = AppState.sensors.find(s => s.id === 'NH3-EFF');
    if (nh3EffSensor && AppState.sensorStatus['NH3-EFF']) {
        document.getElementById('eff-nh3').textContent = AppState.sensorStatus['NH3-EFF'].value.toFixed(2) + ' mg/L';
    }

    const doEffSensor = AppState.sensors.find(s => s.id === 'DO-C-10');
    if (doEffSensor && AppState.sensorStatus['DO-C-10']) {
        document.getElementById('eff-do').textContent = AppState.sensorStatus['DO-C-10'].value.toFixed(2) + ' mg/L';
    }
}

function updateAlarmList(alarms) {
    const alarmList = document.getElementById('alarm-list');
    alarmList.innerHTML = '';

    if (alarms.length === 0) {
        alarmList.innerHTML = '<div class="no-alarm">暂无告警</div>';
        return;
    }

    alarms.forEach(alarm => {
        const item = document.createElement('div');
        item.className = `alarm-item level-${alarm.level}`;
        const time = new Date(alarm.timestamp).toLocaleString('zh-CN');
        item.innerHTML = `
            <div class="alarm-time">${time}</div>
            <div class="alarm-msg">${alarm.message}</div>
        `;
        alarmList.appendChild(item);
    });
}

function drawCanvas() {
    if (!mainCtx) return;

    mainCtx.clearRect(0, 0, mainCanvas.width, mainCanvas.height);

    if (AppState.currentView === 'flow') {
        drawProcessFlow();
    } else {
        drawBiologicalSection();
    }

    drawSensors();
}

function drawProcessFlow() {
    const stages = [
        { id: 'coarse_screen', name: '粗格栅', x: 50, y: 200, w: 80, h: 60, color: '#3498db' },
        { id: 'fine_screen', name: '细格栅', x: 160, y: 200, w: 80, h: 60, color: '#3498db' },
        { id: 'grit_chamber', name: '沉砂池', x: 270, y: 200, w: 80, h: 60, color: '#9b59b6' },
        { id: 'primary_settling', name: '初沉池', x: 380, y: 200, w: 80, h: 60, color: '#1abc9c' },
        { id: 'anaerobic', name: '厌氧池', x: 490, y: 100, w: 100, h: 70, color: '#e74c3c' },
        { id: 'anoxic', name: '缺氧池', x: 490, y: 200, w: 100, h: 70, color: '#f39c12' },
        { id: 'aerobic', name: '好氧池', x: 490, y: 300, w: 100, h: 70, color: '#27ae60' },
        { id: 'secondary_settling', name: '二沉池', x: 620, y: 200, w: 80, h: 60, color: '#1abc9c' },
        { id: 'advanced_treatment', name: '深度处理', x: 730, y: 200, w: 80, h: 60, color: '#34495e' },
        { id: 'effluent', name: '出水', x: 840, y: 200, w: 50, h: 60, color: '#2c3e50' }
    ];

    mainCtx.strokeStyle = '#4a4a7a';
    mainCtx.lineWidth = 3;
    mainCtx.beginPath();
    mainCtx.moveTo(130, 230);
    mainCtx.lineTo(160, 230);
    mainCtx.moveTo(240, 230);
    mainCtx.lineTo(270, 230);
    mainCtx.moveTo(350, 230);
    mainCtx.lineTo(380, 230);
    mainCtx.moveTo(460, 230);
    mainCtx.lineTo(490, 230);
    mainCtx.moveTo(590, 135);
    mainCtx.lineTo(620, 230);
    mainCtx.moveTo(590, 235);
    mainCtx.lineTo(620, 230);
    mainCtx.moveTo(590, 335);
    mainCtx.lineTo(620, 230);
    mainCtx.moveTo(700, 230);
    mainCtx.lineTo(730, 230);
    mainCtx.moveTo(810, 230);
    mainCtx.lineTo(840, 230);
    mainCtx.stroke();

    stages.forEach(stage => {
        drawStageBox(stage);
    });
}

function drawStageBox(stage) {
    const status = getStageStatus(stage.id);

    mainCtx.fillStyle = stage.color;
    mainCtx.fillRect(stage.x, stage.y, stage.w, stage.h);

    if (status === 'alarm') {
        mainCtx.strokeStyle = '#f44336';
        mainCtx.lineWidth = 3;
    } else if (status === 'warning') {
        mainCtx.strokeStyle = '#ff9800';
        mainCtx.lineWidth = 3;
    } else {
        mainCtx.strokeStyle = '#4a4a7a';
        mainCtx.lineWidth = 1;
    }
    mainCtx.strokeRect(stage.x, stage.y, stage.w, stage.h);

    mainCtx.fillStyle = '#fff';
    mainCtx.font = '14px Microsoft YaHei';
    mainCtx.textAlign = 'center';
    mainCtx.fillText(stage.name, stage.x + stage.w / 2, stage.y + stage.h / 2 + 5);
}

function getStageStatus(stageId) {
    let status = 'normal';
    for (const sensor of AppState.sensors) {
        if (sensor.stage === stageId) {
            const s = AppState.sensorStatus[sensor.id];
            if (s) {
                if (!s.online || s.color === '#f44336') {
                    return 'alarm';
                } else if (s.color === '#ff9800') {
                    status = 'warning';
                }
            }
        }
    }
    return status;
}

function drawBiologicalSection() {
    mainCtx.fillStyle = '#2c3e50';
    mainCtx.fillRect(50, 80, 800, 340);

    mainCtx.strokeStyle = '#4a4a7a';
    mainCtx.lineWidth = 2;
    mainCtx.strokeRect(50, 80, 800, 340);

    mainCtx.fillStyle = '#34495e';
    mainCtx.fillRect(50, 80, 150, 340);

    mainCtx.fillStyle = '#8B4513';
    mainCtx.fillRect(200, 80, 650, 340);

    mainCtx.strokeStyle = '#5a5a8a';
    mainCtx.lineWidth = 1;
    mainCtx.setLineDash([5, 5]);
    mainCtx.beginPath();
    mainCtx.moveTo(415, 80);
    mainCtx.lineTo(415, 420);
    mainCtx.moveTo(630, 80);
    mainCtx.lineTo(630, 420);
    mainCtx.stroke();
    mainCtx.setLineDash([]);

    mainCtx.fillStyle = '#e74c3c';
    mainCtx.fillRect(70, 100, 110, 60);
    mainCtx.fillStyle = '#fff';
    mainCtx.font = '16px Microsoft YaHei';
    mainCtx.textAlign = 'center';
    mainCtx.fillText('厌氧区', 125, 138);

    mainCtx.fillStyle = '#f39c12';
    mainCtx.fillRect(70, 180, 110, 60);
    mainCtx.fillStyle = '#fff';
    mainCtx.fillText('缺氧区', 125, 218);

    mainCtx.fillStyle = '#27ae60';
    mainCtx.fillRect(70, 260, 110, 60);
    mainCtx.fillStyle = '#fff';
    mainCtx.fillText('好氧区', 125, 298);

    mainCtx.fillStyle = '#3498db';
    mainCtx.fillRect(70, 340, 110, 60);
    mainCtx.fillStyle = '#fff';
    mainCtx.fillText('沉淀池', 125, 378);

    mainCtx.fillStyle = '#27ae60';
    mainCtx.globalAlpha = 0.3;
    mainCtx.fillRect(220, 100, 610, 280);
    mainCtx.globalAlpha = 1.0;

    mainCtx.fillStyle = '#fff';
    mainCtx.font = 'bold 18px Microsoft YaHei';
    mainCtx.textAlign = 'center';
    mainCtx.fillText('A段', 315, 110);
    mainCtx.fillText('B段', 520, 110);
    mainCtx.fillText('C段', 735, 110);

    mainCtx.font = '12px Microsoft YaHei';
    mainCtx.fillStyle = '#a0a0a0';
    mainCtx.fillText('(曝气区域)', 315, 130);
    mainCtx.fillText('(曝气区域)', 520, 130);
    mainCtx.fillText('(曝气区域)', 735, 130);

    mainCtx.strokeStyle = '#3498db';
    mainCtx.lineWidth = 2;
    for (let i = 0; i < 3; i++) {
        const startX = 250 + i * 215;
        for (let j = 0; j < 5; j++) {
            const bubbleX = startX + j * 35;
            for (let k = 0; k < 3; k++) {
                const bubbleY = 180 + k * 60 + Math.random() * 20;
                const radius = 3 + Math.random() * 4;

                mainCtx.beginPath();
                mainCtx.arc(bubbleX, bubbleY, radius, 0, Math.PI * 2);
                mainCtx.stroke();
            }
        }
    }

    mainCtx.strokeStyle = '#4a4a7a';
    mainCtx.lineWidth = 3;
    mainCtx.setLineDash([]);
    mainCtx.beginPath();
    mainCtx.moveTo(200, 200);
    mainCtx.lineTo(50, 200);
    mainCtx.moveTo(50, 200);
    mainCtx.lineTo(50, 180);
    mainCtx.moveTo(50, 180);
    mainCtx.lineTo(200, 180);
    mainCtx.stroke();

    mainCtx.beginPath();
    mainCtx.moveTo(200, 280);
    mainCtx.lineTo(50, 280);
    mainCtx.moveTo(50, 280);
    mainCtx.lineTo(50, 300);
    mainCtx.moveTo(50, 300);
    mainCtx.lineTo(200, 300);
    mainCtx.stroke();

    mainCtx.fillStyle = '#a0a0a0';
    mainCtx.font = '11px Microsoft YaHei';
    mainCtx.textAlign = 'left';
    mainCtx.fillText('混合液回流', 55, 170);
    mainCtx.fillText('污泥回流', 55, 270);

    mainCtx.fillStyle = '#fff';
    mainCtx.font = '11px Microsoft YaHei';
    mainCtx.textAlign = 'center';
    mainCtx.fillText('进水 →', 30, 200);
    mainCtx.fillText('→ 出水', 870, 380);

    mainCtx.beginPath();
    mainCtx.moveTo(850, 360);
    mainCtx.lineTo(850, 400);
    mainCtx.lineTo(845, 395);
    mainCtx.moveTo(850, 400);
    mainCtx.lineTo(855, 395);
    mainCtx.stroke();
}

function drawSensors() {
    for (const sensor of AppState.sensors) {
        const status = AppState.sensorStatus[sensor.id];
        let color = '#9e9e9e';
        let online = false;

        if (status) {
            color = status.color;
            online = status.online;
        }

        if (AppState.currentView === 'flow') {
            const pos = getFlowViewPosition(sensor);
            if (pos) {
                drawSensorDot(pos.x, pos.y, color, online, sensor.type);
            }
        } else {
            if (['anaerobic', 'anoxic', 'aerobic'].includes(sensor.stage)) {
                const pos = getSectionViewPosition(sensor);
                if (pos) {
                    drawSensorDot(pos.x, pos.y, color, online, sensor.type);
                }
            }
        }
    }
}

function getFlowViewPosition(sensor) {
    const positions = {
        'coarse_screen': { x: 90, y: 180 },
        'fine_screen': { x: 200, y: 180 },
        'grit_chamber': { x: 310, y: 180 },
        'primary_settling': { x: 420, y: 180 },
        'anaerobic': { x: 540, y: 80 },
        'anoxic': { x: 540, y: 180 },
        'aerobic': { x: 540, y: 280 },
        'secondary_settling': { x: 660, y: 180 },
        'advanced_treatment': { x: 770, y: 180 },
        'effluent': { x: 865, y: 180 }
    };

    const base = positions[sensor.stage];
    if (!base) return null;

    const offsetX = (sensor.section - 1) * 20;
    const offsetY = (sensor.id.charCodeAt(sensor.id.length - 1) % 5) * 12;

    return {
        x: base.x + offsetX,
        y: base.y + offsetY
    };
}

function getSectionViewPosition(sensor) {
    const stageOffset = {
        'anaerobic': { x: 220, y: 160 },
        'anoxic': { x: 220, y: 240 },
        'aerobic': { x: 220, y: 320 }
    };

    const base = stageOffset[sensor.stage];
    if (!base) return null;

    let sectionOffsetX = 0;
    if (sensor.section && sensor.section > 1) {
        sectionOffsetX = (sensor.section - 1) * 215;
    }

    return {
        x: base.x + sectionOffsetX + (sensor.x % 100) * 0.5,
        y: base.y + (sensor.id.charCodeAt(sensor.id.length - 1) % 10) * 8
    };
}

function drawSensorDot(x, y, color, online, type) {
    mainCtx.beginPath();
    mainCtx.arc(x, y, 8, 0, Math.PI * 2);

    if (!online) {
        mainCtx.fillStyle = '#9e9e9e';
    } else {
        mainCtx.fillStyle = color;
    }
    mainCtx.fill();

    mainCtx.strokeStyle = '#fff';
    mainCtx.lineWidth = 2;
    mainCtx.stroke();

    if (type) {
        mainCtx.fillStyle = '#a0a0a0';
        mainCtx.font = '10px Microsoft YaHei';
        mainCtx.textAlign = 'center';
        mainCtx.fillText(type, x, y + 20);
    }
}

async function showSensorDetail(sensor, status) {
    AppState.selectedSensor = sensor;

    const modal = document.getElementById('sensor-modal');
    const sensorType = sensorTypeNames[sensor.type] || sensor.type;
    const stageName = stageNames[sensor.stage] || sensor.stage;

    document.getElementById('modal-title').textContent = `${sensorType} - ${sensor.id}`;
    document.getElementById('modal-sensor-id').textContent = sensor.id;
    document.getElementById('modal-sensor-type').textContent = sensorType;
    document.getElementById('modal-sensor-stage').textContent = stageName;
    document.getElementById('modal-sensor-range').textContent = `${sensor.target_min} - ${sensor.target_max} ${sensor.unit}`;

    if (status) {
        document.getElementById('modal-sensor-value').textContent = `${status.value.toFixed(2)} ${sensor.unit}`;
        document.getElementById('modal-sensor-deviation').textContent = `${status.deviation.toFixed(1)}%`;

        let statusText = '正常';
        let statusColor = '#4CAF50';
        if (!status.online) {
            statusText = '离线';
            statusColor = '#9e9e9e';
        } else if (status.color === '#f44336') {
            statusText = '告警';
            statusColor = '#f44336';
        } else if (status.color === '#ff9800') {
            statusText = '警告';
            statusColor = '#ff9800';
        }

        const statusEl = document.getElementById('modal-sensor-status');
        statusEl.textContent = statusText;
        statusEl.style.color = statusColor;
    } else {
        document.getElementById('modal-sensor-value').textContent = '--';
        document.getElementById('modal-sensor-deviation').textContent = '--';
        document.getElementById('modal-sensor-status').textContent = '无数据';
        document.getElementById('modal-sensor-status').style.color = '#9e9e9e';
    }

    modal.classList.add('active');

    try {
        const response = await fetch(`${API_BASE}/sensors/${sensor.id}/trend?hours=6`);
        const data = await response.json();
        if (data.code === 0) {
            drawTrendChart('trend-chart', data.data, sensor.unit);
        }
    } catch (error) {
        console.error('Failed to load sensor trend:', error);
    }
}

function drawTrendChart(canvasId, data, unit) {
    const canvas = document.getElementById(canvasId);
    if (!canvas || !data || data.length === 0) return;

    const ctx = canvas.getContext('2d');
    const width = canvas.width;
    const height = canvas.height;

    ctx.clearRect(0, 0, width, height);

    const padding = { top: 20, right: 20, bottom: 30, left: 50 };
    const chartWidth = width - padding.left - padding.right;
    const chartHeight = height - padding.top - padding.bottom;

    const values = data.map(d => d.value);
    const minVal = Math.min(...values) * 0.9;
    const maxVal = Math.max(...values) * 1.1;
    const valRange = maxVal - minVal || 1;

    ctx.strokeStyle = '#3a3a6a';
    ctx.lineWidth = 1;

    for (let i = 0; i <= 4; i++) {
        const y = padding.top + (chartHeight / 4) * i;
        ctx.beginPath();
        ctx.moveTo(padding.left, y);
        ctx.lineTo(width - padding.right, y);
        ctx.stroke();

        const val = maxVal - (valRange / 4) * i;
        ctx.fillStyle = '#a0a0a0';
        ctx.font = '11px Microsoft YaHei';
        ctx.textAlign = 'right';
        ctx.fillText(val.toFixed(2), padding.left - 5, y + 4);
    }

    ctx.strokeStyle = '#e94560';
    ctx.lineWidth = 2;
    ctx.beginPath();

    data.forEach((point, i) => {
        const x = padding.left + (chartWidth / (data.length - 1 || 1)) * i;
        const y = padding.top + chartHeight - ((point.value - minVal) / valRange) * chartHeight;

        if (i === 0) {
            ctx.moveTo(x, y);
        } else {
            ctx.lineTo(x, y);
        }
    });
    ctx.stroke();

    ctx.fillStyle = 'rgba(233, 69, 96, 0.1)';
    ctx.beginPath();
    data.forEach((point, i) => {
        const x = padding.left + (chartWidth / (data.length - 1 || 1)) * i;
        const y = padding.top + chartHeight - ((point.value - minVal) / valRange) * chartHeight;

        if (i === 0) {
            ctx.moveTo(x, y);
        } else {
            ctx.lineTo(x, y);
        }
    });
    ctx.lineTo(padding.left + chartWidth, padding.top + chartHeight);
    ctx.lineTo(padding.left, padding.top + chartHeight);
    ctx.closePath();
    ctx.fill();

    ctx.fillStyle = '#a0a0a0';
    ctx.font = '11px Microsoft YaHei';
    ctx.textAlign = 'center';

    const timeStep = Math.max(1, Math.floor(data.length / 6));
    for (let i = 0; i < data.length; i += timeStep) {
        const x = padding.left + (chartWidth / (data.length - 1 || 1)) * i;
        const time = new Date(data[i].time);
        const timeStr = `${time.getHours().toString().padStart(2, '0')}:${time.getMinutes().toString().padStart(2, '0')}`;
        ctx.fillText(timeStr, x, height - 10);
    }

    ctx.fillStyle = '#fff';
    ctx.textAlign = 'right';
    ctx.fillText(unit, width - padding.right, padding.top - 5);
}

function drawMiniChart(canvasId, data, color, unit) {
    const canvas = document.getElementById(canvasId);
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    const width = canvas.width;
    const height = canvas.height;

    ctx.clearRect(0, 0, width, height);

    if (!data || data.length < 2) {
        ctx.fillStyle = '#a0a0a0';
        ctx.font = '12px Microsoft YaHei';
        ctx.textAlign = 'center';
        ctx.fillText('暂无数据', width / 2, height / 2);
        return;
    }

    const padding = { top: 5, right: 40, bottom: 5, left: 5 };
    const chartWidth = width - padding.left - padding.right;
    const chartHeight = height - padding.top - padding.bottom;

    const values = data.map(d => d.value);
    const minVal = Math.min(...values);
    const maxVal = Math.max(...values);
    const valRange = maxVal - minVal || 1;

    ctx.strokeStyle = color;
    ctx.lineWidth = 2;
    ctx.beginPath();

    data.forEach((point, i) => {
        const x = padding.left + (chartWidth / (data.length - 1 || 1)) * i;
        const y = padding.top + chartHeight - ((point.value - minVal) / valRange) * chartHeight;

        if (i === 0) {
            ctx.moveTo(x, y);
        } else {
            ctx.lineTo(x, y);
        }
    });
    ctx.stroke();

    ctx.fillStyle = color;
    ctx.globalAlpha = 0.2;
    ctx.beginPath();
    data.forEach((point, i) => {
        const x = padding.left + (chartWidth / (data.length - 1 || 1)) * i;
        const y = padding.top + chartHeight - ((point.value - minVal) / valRange) * chartHeight;

        if (i === 0) {
            ctx.moveTo(x, y);
        } else {
            ctx.lineTo(x, y);
        }
    });
    ctx.lineTo(padding.left + chartWidth, padding.top + chartHeight);
    ctx.lineTo(padding.left, padding.top + chartHeight);
    ctx.closePath();
    ctx.fill();
    ctx.globalAlpha = 1.0;

    const lastVal = values[values.length - 1];
    ctx.fillStyle = color;
    ctx.font = 'bold 14px Microsoft YaHei';
    ctx.textAlign = 'right';
    ctx.fillText(lastVal.toFixed(3), width - 5, height / 2 + 5);

    ctx.fillStyle = '#a0a0a0';
    ctx.font = '10px Microsoft YaHei';
    ctx.fillText(unit, width - 5, height / 2 + 20);

    if (values.length >= 2) {
        const prevVal = values[values.length - 2];
        const trend = lastVal - prevVal;
        const trendEl = document.getElementById('trend-' + canvasId.split('-')[1]);
        if (trendEl) {
            if (Math.abs(trend) < 0.001) {
                trendEl.textContent = '→ 稳定';
                trendEl.className = 'kpi-trend stable';
            } else if (trend > 0) {
                trendEl.textContent = '↑ 上升';
                trendEl.className = 'kpi-trend up';
            } else {
                trendEl.textContent = '↓ 下降';
                trendEl.className = 'kpi-trend down';
            }
        }
    }
}
