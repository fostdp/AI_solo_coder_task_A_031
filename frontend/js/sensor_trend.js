class SensorTrend {
    constructor(canvasId, options = {}) {
        this.canvas = document.getElementById(canvasId);
        if (!this.canvas) {
            console.error(`Canvas element not found: ${canvasId}`);
            return;
        }
        this.ctx = this.canvas.getContext('2d');
        this.options = Object.assign({
            hours: 6,
            color: CONFIG.COLORS.primary,
            label: '数值',
            unit: '',
            showGrid: true,
            showPoints: true,
            smooth: true
        }, options);
        
        this.data = {
            timestamps: [],
            values: []
        };
        
        this.sensorId = options.sensorId;
        this.sensorType = options.sensorType;
        
        this.init();
    }
    
    init() {
        this.resize();
        if (this.sensorId) {
            this.loadData();
        }
    }
    
    resize() {
        const container = this.canvas.parentElement;
        if (container) {
            this.canvas.width = container.clientWidth;
            this.canvas.height = container.clientHeight || 300;
        }
        this.draw();
    }
    
    async loadData(hours = this.options.hours) {
        if (!this.sensorId) return;
        
        try {
            const response = await fetch(`${CONFIG.API_BASE_URL}/sensors/${this.sensorId}/trend?hours=${hours}`);
            const data = await response.json();
            this.setData(data.timestamps, data.values);
        } catch (error) {
            console.error('Failed to load sensor trend:', error);
            this.generateMockData(hours);
        }
    }
    
    generateMockData(hours = 6) {
        const timestamps = [];
        const values = [];
        const baseValue = this.sensorType ? 
            CONFIG.SENSOR_TYPES[this.sensorType]?.setpoint || 1.0 : 1.0;
        const points = hours * 30;
        
        for (let i = 0; i < points; i++) {
            timestamps.push(new Date(Date.now() - (points - i) * 2 * 60 * 1000));
            const variation = (Math.random() - 0.5) * baseValue * 0.2;
            values.push(baseValue + variation);
        }
        
        this.setData(timestamps, values);
    }
    
    setData(timestamps, values) {
        this.data.timestamps = timestamps.map(t => new Date(t));
        this.data.values = values;
        this.draw();
    }
    
    addDataPoint(timestamp, value) {
        this.data.timestamps.push(new Date(timestamp));
        this.data.values.push(value);
        
        const maxPoints = this.options.hours * 30;
        if (this.data.timestamps.length > maxPoints) {
            this.data.timestamps = this.data.timestamps.slice(-maxPoints);
            this.data.values = this.data.values.slice(-maxPoints);
        }
        
        this.draw();
    }
    
    draw() {
        if (!this.ctx || this.data.values.length === 0) return;
        
        const ctx = this.ctx;
        const w = this.canvas.width;
        const h = this.canvas.height;
        const padding = { top: 40, right: 60, bottom: 50, left: 60 };
        const chartWidth = w - padding.left - padding.right;
        const chartHeight = h - padding.top - padding.bottom;
        
        ctx.clearRect(0, 0, w, h);
        
        this.drawBackground(ctx, w, h);
        
        if (this.options.showGrid) {
            this.drawGrid(ctx, padding, chartWidth, chartHeight);
        }
        
        this.drawAxes(ctx, padding, chartWidth, chartHeight);
        
        this.drawTrend(ctx, padding, chartWidth, chartHeight);
        
        if (this.options.showPoints) {
            this.drawPoints(ctx, padding, chartWidth, chartHeight);
        }
        
        this.drawTitle(ctx, padding, w);
        
        this.drawLatestValue(ctx, padding, chartWidth, chartHeight);
    }
    
    drawBackground(ctx, w, h) {
        ctx.fillStyle = '#1e293b';
        ctx.fillRect(0, 0, w, h);
    }
    
    drawGrid(ctx, padding, chartWidth, chartHeight) {
        ctx.strokeStyle = 'rgba(71, 85, 105, 0.3)';
        ctx.lineWidth = 1;
        
        for (let i = 0; i <= 5; i++) {
            const y = padding.top + (chartHeight / 5) * i;
            ctx.beginPath();
            ctx.moveTo(padding.left, y);
            ctx.lineTo(padding.left + chartWidth, y);
            ctx.stroke();
        }
    }
    
    drawAxes(ctx, padding, chartWidth, chartHeight) {
        const values = this.data.values;
        const minVal = Math.min(...values) * 0.9;
        const maxVal = Math.max(...values) * 1.1;
        
        ctx.fillStyle = '#94a3b8';
        ctx.font = '11px Microsoft YaHei';
        ctx.textAlign = 'right';
        
        for (let i = 0; i <= 5; i++) {
            const y = padding.top + (chartHeight / 5) * i;
            const val = maxVal - (maxVal - minVal) * (i / 5);
            ctx.fillText(val.toFixed(2), padding.left - 10, y + 4);
        }
        
        ctx.textAlign = 'center';
        const timestamps = this.data.timestamps;
        const labelCount = Math.min(6, timestamps.length);
        for (let i = 0; i < labelCount; i++) {
            const idx = Math.floor(i * (timestamps.length - 1) / (labelCount - 1));
            const x = padding.left + (chartWidth / (labelCount - 1)) * i;
            const time = timestamps[idx];
            ctx.fillText(
                time.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }),
                x,
                padding.top + chartHeight + 25
            );
        }
        
        ctx.fillStyle = '#64748b';
        ctx.font = '12px Microsoft YaHei';
        ctx.save();
        ctx.translate(20, padding.top + chartHeight / 2);
        ctx.rotate(-Math.PI / 2);
        ctx.fillText(this.options.unit, 0, 0);
        ctx.restore();
    }
    
    drawTrend(ctx, padding, chartWidth, chartHeight) {
        const values = this.data.values;
        const minVal = Math.min(...values) * 0.9;
        const maxVal = Math.max(...values) * 1.1;
        const len = values.length;
        
        const gradient = ctx.createLinearGradient(0, padding.top, 0, padding.top + chartHeight);
        gradient.addColorStop(0, this.options.color + '40');
        gradient.addColorStop(1, this.options.color + '00');
        
        ctx.beginPath();
        ctx.moveTo(padding.left, padding.top + chartHeight);
        
        if (this.options.smooth && len > 2) {
            const getY = (val) => padding.top + chartHeight - ((val - minVal) / (maxVal - minVal)) * chartHeight;
            
            ctx.lineTo(padding.left, getY(values[0]));
            
            for (let i = 0; i < len - 1; i++) {
                const x0 = padding.left + (chartWidth / (len - 1)) * i;
                const x1 = padding.left + (chartWidth / (len - 1)) * (i + 1);
                const y0 = getY(values[i]);
                const y1 = getY(values[i + 1]);
                
                const cpx = (x0 + x1) / 2;
                ctx.quadraticCurveTo(cpx, y0, cpx, (y0 + y1) / 2);
                ctx.quadraticCurveTo(cpx, y1, x1, y1);
            }
        } else {
            for (let i = 0; i < len; i++) {
                const x = padding.left + (chartWidth / (len - 1)) * i;
                const y = padding.top + chartHeight - ((values[i] - minVal) / (maxVal - minVal)) * chartHeight;
                
                if (i === 0) {
                    ctx.lineTo(x, y);
                } else {
                    ctx.lineTo(x, y);
                }
            }
        }
        
        ctx.lineTo(padding.left + chartWidth, padding.top + chartHeight);
        ctx.closePath();
        ctx.fillStyle = gradient;
        ctx.fill();
        
        ctx.beginPath();
        for (let i = 0; i < len; i++) {
            const x = padding.left + (chartWidth / (len - 1)) * i;
            const y = padding.top + chartHeight - ((values[i] - minVal) / (maxVal - minVal)) * chartHeight;
            
            if (i === 0) {
                ctx.moveTo(x, y);
            } else {
                ctx.lineTo(x, y);
            }
        }
        ctx.strokeStyle = this.options.color;
        ctx.lineWidth = 2;
        ctx.stroke();
    }
    
    drawPoints(ctx, padding, chartWidth, chartHeight) {
        const values = this.data.values;
        const minVal = Math.min(...values) * 0.9;
        const maxVal = Math.max(...values) * 1.1;
        const len = values.length;
        const step = Math.max(1, Math.floor(len / 20));
        
        for (let i = 0; i < len; i += step) {
            const x = padding.left + (chartWidth / (len - 1)) * i;
            const y = padding.top + chartHeight - ((values[i] - minVal) / (maxVal - minVal)) * chartHeight;
            
            ctx.beginPath();
            ctx.arc(x, y, 3, 0, Math.PI * 2);
            ctx.fillStyle = this.options.color;
            ctx.fill();
            ctx.strokeStyle = '#fff';
            ctx.lineWidth = 1;
            ctx.stroke();
        }
    }
    
    drawTitle(ctx, padding, w) {
        ctx.fillStyle = '#e2e8f0';
        ctx.font = 'bold 14px Microsoft YaHei';
        ctx.textAlign = 'left';
        ctx.fillText(this.options.label, padding.left, 20);
    }
    
    drawLatestValue(ctx, padding, chartWidth, chartHeight) {
        const values = this.data.values;
        const lastValue = values[values.length - 1];
        const avgValue = values.reduce((a, b) => a + b, 0) / values.length;
        
        ctx.fillStyle = '#94a3b8';
        ctx.font = '12px Microsoft YaHei';
        ctx.textAlign = 'right';
        ctx.fillText(
            `最新值: ${lastValue.toFixed(3)} ${this.options.unit}`,
            padding.left + chartWidth,
            20
        );
        ctx.fillText(
            `平均值: ${avgValue.toFixed(3)} ${this.options.unit}`,
            padding.left + chartWidth,
            38
        );
    }
    
    setOptions(options) {
        this.options = Object.assign(this.options, options);
        this.draw();
    }
    
    clear() {
        this.data = { timestamps: [], values: [] };
        this.draw();
    }
}

async function drawSensorTrend(sensorId) {
    try {
        const response = await fetch(`${CONFIG.API_BASE_URL}/sensors/${sensorId}/trend?hours=6`);
        const data = await response.json();
        
        const canvas = document.getElementById('modalTrendCanvas');
        const ctx = canvas.getContext('2d');
        
        const container = canvas.parentElement;
        canvas.width = container.clientWidth;
        canvas.height = 300;
        
        const sensorType = sensorId.split('-')[0];
        const color = CONFIG.SENSOR_TYPES[sensorType]?.color || CONFIG.COLORS.primary;
        const label = CONFIG.SENSOR_TYPES[sensorType]?.name || '数值';
        const unit = CONFIG.SENSOR_TYPES[sensorType]?.unit || '';
        
        drawLineChart(ctx, canvas, data.timestamps, data.values, {
            color: color,
            label: label,
            unit: unit
        });
    } catch (error) {
        console.error('Failed to load sensor trend:', error);
    }
}

function drawLineChart(ctx, canvas, timestamps, values, options = {}) {
    const w = canvas.width;
    const h = canvas.height;
    const padding = { top: 40, right: 80, bottom: 50, left: 60 };
    const chartWidth = w - padding.left - padding.right;
    const chartHeight = h - padding.top - padding.bottom;
    
    ctx.clearRect(0, 0, w, h);
    
    ctx.fillStyle = '#1e293b';
    ctx.fillRect(0, 0, w, h);
    
    if (!values || values.length === 0) return;
    
    const minVal = Math.min(...values) * 0.9;
    const maxVal = Math.max(...values) * 1.1;
    const len = values.length;
    
    ctx.strokeStyle = 'rgba(71, 85, 105, 0.3)';
    ctx.lineWidth = 1;
    for (let i = 0; i <= 5; i++) {
        const y = padding.top + (chartHeight / 5) * i;
        ctx.beginPath();
        ctx.moveTo(padding.left, y);
        ctx.lineTo(padding.left + chartWidth, y);
        ctx.stroke();
        
        ctx.fillStyle = '#94a3b8';
        ctx.font = '11px Microsoft YaHei';
        ctx.textAlign = 'right';
        const val = maxVal - (maxVal - minVal) * (i / 5);
        ctx.fillText(val.toFixed(2), padding.left - 10, y + 4);
    }
    
    ctx.fillStyle = '#64748b';
    ctx.font = '12px Microsoft YaHei';
    ctx.textAlign = 'center';
    const labelCount = Math.min(6, len);
    for (let i = 0; i < labelCount; i++) {
        const idx = Math.floor(i * (len - 1) / (labelCount - 1));
        const x = padding.left + (chartWidth / (labelCount - 1)) * i;
        const time = new Date(timestamps[idx]);
        ctx.fillText(
            time.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }),
            x,
            padding.top + chartHeight + 25
        );
    }
    
    const gradient = ctx.createLinearGradient(0, padding.top, 0, padding.top + chartHeight);
    gradient.addColorStop(0, options.color + '40');
    gradient.addColorStop(1, options.color + '00');
    
    const getY = (val) => padding.top + chartHeight - ((val - minVal) / (maxVal - minVal)) * chartHeight;
    
    ctx.beginPath();
    ctx.moveTo(padding.left, padding.top + chartHeight);
    
    if (len > 2) {
        ctx.lineTo(padding.left, getY(values[0]));
        
        for (let i = 0; i < len - 1; i++) {
            const x0 = padding.left + (chartWidth / (len - 1)) * i;
            const x1 = padding.left + (chartWidth / (len - 1)) * (i + 1);
            const y0 = getY(values[i]);
            const y1 = getY(values[i + 1]);
            
            const cpx = (x0 + x1) / 2;
            ctx.quadraticCurveTo(cpx, y0, cpx, (y0 + y1) / 2);
            ctx.quadraticCurveTo(cpx, y1, x1, y1);
        }
    } else {
        for (let i = 0; i < len; i++) {
            const x = padding.left + (chartWidth / Math.max(1, len - 1)) * i;
            ctx.lineTo(x, getY(values[i]));
        }
    }
    
    ctx.lineTo(padding.left + chartWidth, padding.top + chartHeight);
    ctx.closePath();
    ctx.fillStyle = gradient;
    ctx.fill();
    
    ctx.beginPath();
    for (let i = 0; i < len; i++) {
        const x = padding.left + (chartWidth / (len - 1)) * i;
        const y = getY(values[i]);
        
        if (i === 0) {
            ctx.moveTo(x, y);
        } else {
            ctx.lineTo(x, y);
        }
    }
    ctx.strokeStyle = options.color;
    ctx.lineWidth = 2;
    ctx.stroke();
    
    const step = Math.max(1, Math.floor(len / 20));
    for (let i = 0; i < len; i += step) {
        const x = padding.left + (chartWidth / (len - 1)) * i;
        const y = getY(values[i]);
        
        ctx.beginPath();
        ctx.arc(x, y, 3, 0, Math.PI * 2);
        ctx.fillStyle = options.color;
        ctx.fill();
        ctx.strokeStyle = '#fff';
        ctx.lineWidth = 1;
        ctx.stroke();
    }
    
    ctx.fillStyle = '#e2e8f0';
    ctx.font = 'bold 14px Microsoft YaHei';
    ctx.textAlign = 'left';
    ctx.fillText(options.label || '趋势曲线', padding.left, 20);
    
    const lastValue = values[len - 1];
    const avgValue = values.reduce((a, b) => a + b, 0) / len;
    
    ctx.fillStyle = '#94a3b8';
    ctx.font = '12px Microsoft YaHei';
    ctx.textAlign = 'right';
    ctx.fillText(
        `最新值: ${lastValue.toFixed(3)} ${options.unit || ''}`,
        padding.left + chartWidth,
        20
    );
    ctx.fillText(
        `平均值: ${avgValue.toFixed(3)} ${options.unit || ''}`,
        padding.left + chartWidth,
        38
    );
}
