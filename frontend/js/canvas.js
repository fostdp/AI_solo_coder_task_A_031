const CanvasRenderer = {
    sensors: [],
    sensorData: {},
    profileCanvas: null,
    planCanvas: null,
    profileCtx: null,
    planCtx: null,
    onSensorClick: null,

    init(onSensorClick) {
        this.onSensorClick = onSensorClick;
        this.profileCanvas = document.getElementById('profile-canvas');
        this.planCanvas = document.getElementById('plan-canvas');
        this.profileCtx = this.profileCanvas.getContext('2d');
        this.planCtx = this.planCanvas.getContext('2d');

        this.profileCanvas.addEventListener('click', (e) => this.handleProfileClick(e));
        this.planCanvas.addEventListener('click', (e) => this.handlePlanClick(e));

        this.drawProfile();
        this.drawPlan();
    },

    updateSensors(sensors) {
        this.sensors = sensors;
        this.sensorData = {};
        sensors.forEach(s => {
            this.sensorData[s.id] = s;
        });
        this.redrawAll();
    },

    updateSensorData(sensor) {
        this.sensorData[sensor.id] = sensor;
        this.redrawAll();
    },

    redrawAll() {
        this.drawProfile();
        this.drawPlan();
    },

    drawProfile() {
        const ctx = this.profileCtx;
        const width = this.profileCanvas.width;
        const height = this.profileCanvas.height;

        ctx.clearRect(0, 0, width, height);

        const gradient = ctx.createLinearGradient(0, 0, 0, height);
        gradient.addColorStop(0, '#0a1628');
        gradient.addColorStop(1, '#1a2a4a');
        ctx.fillStyle = gradient;
        ctx.fillRect(0, 0, width, height);

        this.drawGrid(ctx, width, height);
        this.drawBiologicalProfile(ctx, width, height);
        this.drawProfileSensors(ctx);
    },

    drawGrid(ctx, width, height) {
        ctx.strokeStyle = 'rgba(0, 212, 255, 0.1)';
        ctx.lineWidth = 1;

        for (let x = 0; x < width; x += 50) {
            ctx.beginPath();
            ctx.moveTo(x, 0);
            ctx.lineTo(x, height);
            ctx.stroke();
        }
        for (let y = 0; y < height; y += 50) {
            ctx.beginPath();
            ctx.moveTo(0, y);
            ctx.lineTo(width, y);
            ctx.stroke();
        }
    },

    drawBiologicalProfile(ctx, width, height) {
        const sections = [
            { name: '厌氧池', y: 50, height: 120, color: 'rgba(155, 89, 182, 0.3)', border: '#9b59b6' },
            { name: '缺氧池', y: 220, height: 200, color: 'rgba(52, 152, 219, 0.3)', border: '#3498db' },
            { name: '好氧池(6廊道)', y: 470, height: 520, color: 'rgba(46, 204, 113, 0.2)', border: '#2ecc71' },
        ];

        sections.forEach((section, idx) => {
            ctx.fillStyle = section.color;
            ctx.strokeStyle = section.border;
            ctx.lineWidth = 2;
            ctx.lineWidth = 2;

            this.drawTank(ctx, 100, section.y, width - 200, section.height, section.color, section.border);

            ctx.fillStyle = '#fff';
            ctx.font = 'bold 16px Microsoft YaHei';
            ctx.textAlign = 'left';
            ctx.fillText(section.name, 120, section.y + 30);

            if (section.name.includes('好氧')) {
                for (let i = 0; i < 6; i++) {
                    const corrX = 150 + i * 130;
                    this.drawAerator(ctx, corrX, section.y + section.height - 50);
                }
            }
        });

        const inX = 30;
        const outX = width - 170;

        ctx.fillStyle = 'rgba(0, 0, 0, 0.5)';
        ctx.fillRect(inX, 50, 50, height - 100);
        ctx.strokeStyle = '#666';
        ctx.strokeRect(inX, 50, 50, height - 100);
        ctx.fillStyle = '#aaa';
        ctx.font = '12px Microsoft YaHei';
        ctx.textAlign = 'center';
        ctx.fillText('进水', inX + 25, 40);

        ctx.fillStyle = 'rgba(0, 0, 0, 0.5)';
        ctx.fillRect(outX, 50, 50, height - 100);
        ctx.strokeStyle = '#666';
        ctx.strokeRect(outX, 50, 50, height - 100);
        ctx.fillStyle = '#aaa';
        ctx.fillText('出水', outX + 25, 40);

        this.drawWaterFlow(ctx, inX + 50, height / 2, 80, 20);
        this.drawWaterFlow(ctx, outX - 80, height / 2, 80, -20);

        const depths = [0, 2, 4, 6, 8];
        ctx.fillStyle = '#888';
        ctx.font = '11px Microsoft YaHei';
        ctx.textAlign = 'right';
        depths.forEach((d, i) => {
            const yPos = 50 + i * (height - 100) / 4;
            ctx.fillText(d + 'm', 90, yPos);
        });
    },

    drawTank(ctx, x, y, w, h, fill, border) {
        ctx.fillStyle = fill;
        ctx.fillRect(x, y, w, h);

        ctx.strokeStyle = border;
        ctx.lineWidth = 2;
        ctx.strokeRect(x, y, w, h);

        const waterGradient = ctx.createLinearGradient(x, y, x, y + h);
        waterGradient.addColorStop(0, 'rgba(100, 200, 255, 0.4)');
        waterGradient.addColorStop(1, 'rgba(50, 100, 150, 0.6)');
        ctx.fillStyle = waterGradient;
        ctx.fillRect(x + 5, y + 25, w - 10, h - 30);

        ctx.strokeStyle = 'rgba(255, 255, 255, 0.3)';
        ctx.lineWidth = 1;
        const time = Date.now() / 1000;
        for (let i = 0; i < 3; i++) {
            ctx.beginPath();
            for (let wx = x + 10; wx < x + w - 10; wx += 3) {
                const wy = y + 35 + Math.sin(wx * 0.05 + time + i) * 3;
                if (wx === x + 10) ctx.moveTo(wx, wy);
                else ctx.lineTo(wx, wy);
            }
            ctx.stroke();
        }
    },

    drawAerator(ctx, x, y) {
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.6)';
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.moveTo(x, y);
        ctx.lineTo(x, y + 30);
        ctx.stroke();

        const bubbleCount = 5 + Math.floor(Math.random() * 3);
        for (let i = 0; i < bubbleCount; i++) {
            const bx = x - 10 + Math.random() * 20;
            const by = y - 10 - Math.random() * 80;
            const br = 2 + Math.random() * 3;
            ctx.beginPath();
            ctx.arc(bx, by, br, 0, Math.PI * 2);
            ctx.fillStyle = `rgba(200, 230, 255, ${0.3 + Math.random() * 0.4})';
            ctx.fill();
        }
    },

    drawWaterFlow(ctx, x, y, length, dir) {
        ctx.strokeStyle = 'rgba(0, 212, 255, 0.6)';
        ctx.lineWidth = 3;
        ctx.beginPath();
        ctx.moveTo(x, y);
        ctx.lineTo(x + length * dir, y);
        ctx.stroke();

        ctx.beginPath();
        ctx.moveTo(x + length * dir, y);
        ctx.lineTo(x + length * dir - 10 * dir, y - 5);
        ctx.moveTo(x + length * dir, y);
        ctx.lineTo(x + length * dir - 10 * dir, y + 5);
        ctx.stroke();
    },

    drawProfileSensors(ctx) {
        this.sensors.forEach(sensor => {
            if (sensor.x && sensor.y) {
                this.drawSensorDot(ctx, sensor);
            }
        });
    },

    drawSensorDot(ctx, sensor) {
        const x = sensor.x;
        const y = sensor.y;
        const color = getSensorColor(sensor);
        const isOffline = color === CONFIG.COLORS.offline;
        const radius = isOffline ? 6 : 8;

        const gradient = ctx.createRadialGradient(x, y, 0, x, y, radius * 2);
        gradient.addColorStop(0, color);
        gradient.addColorStop(1, 'transparent');
        ctx.fillStyle = gradient;
        ctx.beginPath();
        ctx.arc(x, y, radius * 2, 0, Math.PI * 2);
        ctx.fill();

        ctx.fillStyle = color;
        ctx.beginPath();
        ctx.arc(x, y, radius, 0, Math.PI * 2);
        ctx.fill();

        ctx.strokeStyle = '#fff';
        ctx.lineWidth = 2;
        ctx.stroke();

        if (!isOffline && sensor.value !== undefined) {
            ctx.fillStyle = '#fff';
            ctx.font = 'bold 10px Microsoft YaHei';
            ctx.textAlign = 'center';
            ctx.fillText(formatValue(sensor.value, sensor.type), x, y - radius - 5);
        }

        ctx.fillStyle = '#aaa';
        ctx.font = '9px Microsoft YaHei';
        ctx.fillText(sensor.id, x, y + radius + 12);
    },

    handleProfileClick(e) {
        const rect = this.profileCanvas.getBoundingClientRect();
        const scaleX = this.profileCanvas.width / rect.width;
        const scaleY = this.profileCanvas.height / rect.height;
        const x = (e.clientX - rect.left) * scaleX;
        const y = (e.clientY - rect.top) * scaleY;

        this.handleCanvasClick(x, y);
    },

    handlePlanClick(e) {
        const rect = this.planCanvas.getBoundingClientRect();
        const scaleX = this.planCanvas.width / rect.width;
        const scaleY = this.planCanvas.height / rect.height;
        const x = (e.clientX - rect.left) * scaleX;
        const y = (e.clientY - rect.top) * scaleY;

        this.handleCanvasClick(x, y);
    },

    handleCanvasClick(x, y) {
        for (const sensor of this.sensors) {
            if (!sensor.x || !sensor.y) continue;
            const dx = x - sensor.x;
            const dy = y - sensor.y;
            const distance = Math.sqrt(dx * dx + dy * dy);
            if (distance < 15) {
                if (this.onSensorClick) {
                    this.onSensorClick(sensor);
                }
                return;
            }
        }
    },

    drawPlan() {
        const ctx = this.planCtx;
        const width = this.planCanvas.width;
        const height = this.planCanvas.height;

        ctx.clearRect(0, 0, width, height);

        const gradient = ctx.createLinearGradient(0, 0, 0, height);
        gradient.addColorStop(0, '#0a1628');
        gradient.addColorStop(1, '#1a2a4a');
        ctx.fillStyle = gradient;
        ctx.fillRect(0, 0, width, height);

        this.drawPlanGrid(ctx, width, height);

        const stages = [
            { name: '粗格栅', x: 30, y: 50, w: 100, h: 80, color: '#e74c3c' },
            { name: '细格栅', x: 150, y: 50, w: 100, h: 80, color: '#e67e22' },
            { name: '沉砂池', x: 270, y: 50, w: 100, h: 80, color: '#f39c12' },
            { name: '初沉池', x: 390, y: 50, w: 120, h: 80, color: '#f1c40f' },
            { name: '厌氧池', x: 50, y: 200, w: 130, h: 120, color: '#9b59b6' },
            { name: '缺氧池', x: 200, y: 200, w: 130, h: 120, color: '#3498db' },
            { name: '好氧池', x: 350, y: 200, w: 300, h: 120, color: '#2ecc71' },
            { name: '二沉池', x: 670, y: 200, w: 120, h: 120, color: '#1abc9c' },
            { name: '深度处理', x: 810, y: 200, w: 120, h: 120, color: '#16a085' },
            { name: '出水', x: 810, y: 50, w: 120, h: 80, color: '#34495e' },
        ];

        stages.forEach(stage => {
            this.drawProcessStage(ctx, stage);
        });

        this.drawArrows(ctx);

        this.drawPlanLabels(ctx, width, height);

        this.drawPlanSensors(ctx);
    },

    drawPlanGrid(ctx, width, height) {
        ctx.strokeStyle = 'rgba(0, 212, 255, 0.1)';
        ctx.lineWidth = 1;
        for (let x = 0; x < width; x += 50) {
            ctx.beginPath();
            ctx.moveTo(x, 0);
            ctx.lineTo(x, height);
            ctx.stroke();
        }
        for (let y = 0; y < height; y += 50) {
            ctx.beginPath();
            ctx.moveTo(0, y);
            ctx.lineTo(width, y);
            ctx.stroke();
        }
    },

    drawProcessStage(ctx, stage) {
        ctx.fillStyle = `${stage.color}33';
        ctx.fillRect(stage.x, stage.y, stage.w, stage.h);

        ctx.strokeStyle = stage.color;
        ctx.lineWidth = 2;
        ctx.strokeRect(stage.x, stage.y, stage.w, stage.h);

        ctx.fillStyle = '#fff';
        ctx.font = 'bold 14px Microsoft YaHei';
        ctx.textAlign = 'center';
        ctx.fillText(stage.name, stage.x + stage.w / 2, stage.y + stage.h / 2 + 5);

        ctx.fillStyle = '#aaa';
        ctx.font = '10px Microsoft YaHei';
    },

    drawArrows(ctx) {
        const arrowPositions = [
            { x1: 130, y1: 90, x2: 150, y2: 90 },
            { x1: 250, y1: 90, x2: 270, y2: 90 },
            { x1: 370, y1: 90, x2: 390, y2: 90 },
            { x1: 510, y1: 90, x2: 530, y2: 90 },
            { x1: 450, y1: 130, x2: 450, y2: 200 },
            { x1: 115, y1: 170, x2: 115, y2: 200 },
            { x1: 180, y1: 260, x2: 200, y2: 260 },
            { x1: 330, y1: 260, x2: 350, y2: 260 },
            { x1: 650, y1: 260, x2: 670, y2: 260 },
            { x1: 790, y1: 260, x2: 810, y2: 260 },
            { x1: 870, y1: 170, x2: 870, y2: 200 },
            { x1: 650, y1: 90, x2: 670, y2: 90 },
        ];

        ctx.strokeStyle = 'rgba(0, 212, 255, 0.6)';
        ctx.lineWidth = 2;

        arrowPositions.forEach(pos => {
            ctx.beginPath();
            ctx.moveTo(pos.x1, pos.y1);
            ctx.lineTo(pos.x2, pos.y2);
            ctx.stroke();

            const angle = Math.atan2(pos.y2 - pos.y1, pos.x2 - pos.x1);
            const arrowSize = 6;
            ctx.beginPath();
            ctx.moveTo(pos.x2, pos.y2);
            ctx.lineTo(pos.x2 - arrowSize * Math.cos(angle - Math.PI / 6), pos.y2 - arrowSize * Math.sin(angle - Math.PI / 6));
            ctx.moveTo(pos.x2, pos.y2);
            ctx.lineTo(pos.x2 - arrowSize * Math.cos(angle + Math.PI / 6), pos.y2 - arrowSize * Math.sin(angle + Math.PI / 6));
            ctx.stroke();
        });
    },

    drawPlanLabels(ctx, width, height) {
        ctx.fillStyle = '#888';
        ctx.font = '11px Microsoft YaHei';
        ctx.textAlign = 'left';
        ctx.fillText('进水方向 →', 30, 30);

        ctx.fillStyle = '#00d4ff';
        ctx.font = 'bold 18px Microsoft YaHei';
        ctx.textAlign = 'center';
        ctx.fillText('污水处理工艺流程图', width / 2, height - 20);
    },

    drawPlanSensors(ctx) {
        const planSensorPositions = {
            'FLOW-INF-01': { x: 80, y: 90 },
            'COD-INF-01': { x: 450, y: 90 },
            'PO4-AER-01': { x: 115, y: 260 },
            'NO3-ANX-01': { x: 265, y: 260 },
            'DO-AER-01': { x: 500, y: 260 },
            'NH3-AER-01': { x: 570, y: 260 },
            'DO-AER-15': { x: 450, y: 260 },
            'NH3-EFF-01': { x: 870, y: 70 },
            'TN-EFF-01': { x: 870, y: 90 },
            'TP-EFF-01': { x: 870, y: 110 },
        };

        Object.keys(planSensorPositions).forEach(sensorId => {
            const sensor = this.sensorData[sensorId];
            const pos = planSensorPositions[sensorId];
            if (sensor && pos) {
                const displaySensor = { ...sensor, x: pos.x, y: pos.y };
                this.drawSensorDot(ctx, displaySensor);
            }
        });
    },
};
