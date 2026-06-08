const TrendChart = {
    canvas: null,
    ctx: null,
    metricsCanvas: null,
    metricsCtx: null,

    init() {
        this.canvas = document.getElementById('trend-canvas');
        this.ctx = this.canvas.getContext('2d');
        this.metricsCanvas = document.getElementById('metrics-trend-canvas');
        this.metricsCtx = this.metricsCanvas.getContext('2d');
    },

    drawSensorTrend(data, sensorInfo) {
        const ctx = this.ctx;
        const width = this.canvas.width;
        const height = this.canvas.height;

        ctx.clearRect(0, 0, width, height);

        const bgGradient = ctx.createLinearGradient(0, 0, 0, height);
        bgGradient.addColorStop(0, '#0f192a');
        bgGradient.addColorStop(1, '#1a2a4a');
        ctx.fillStyle = bgGradient;
        ctx.fillRect(0, 0, width, height);

        if (!data || data.length === 0) {
            ctx.fillStyle = '#888';
            ctx.font = '14px Microsoft YaHei';
            ctx.textAlign = 'center';
            ctx.fillText('暂无数据', width / 2, height / 2);
            return;
        }

        const padding = { top: 40, right: 60, bottom: 50, left: 60 };
        const chartWidth = width - padding.left - padding.right;
        const chartHeight = height - padding.top - padding.bottom;

        const values = data.map(d => d.value);
        const minVal = Math.min(...values);
        const maxVal = Math.max(...values);
        const range = maxVal - minVal || 1;
        const yMin = minVal - range * 0.1;
        const yMax = maxVal + range * 0.1;

        this.drawGrid(ctx, padding, chartWidth, chartHeight, yMin, yMax, data);
        this.drawYAxis(ctx, padding, chartHeight, yMin, yMax);
        this.drawXAxis(ctx, padding, chartWidth, chartHeight, data);

        this.drawTrendLine(ctx, data, padding, chartWidth, chartHeight, yMin, yMax);
        this.drawSetpointLine(ctx, sensorInfo, padding, chartHeight, yMin, yMax);

        this.drawDataPoints(ctx, data, padding, chartWidth, chartHeight, yMin, yMax);
    },

    drawGrid(ctx, padding, chartWidth, chartHeight, yMin, yMax) {
        ctx.strokeStyle = 'rgba(0, 212, 255, 0.15)';
        ctx.lineWidth = 1;

        for (let i = 0; i <= 5; i++) {
            const y = padding.top + (chartHeight / 5) * i;
            ctx.beginPath();
            ctx.moveTo(padding.left, y);
            ctx.lineTo(padding.left + chartWidth, y);
            ctx.stroke();

            const value = yMax - (yMax - yMin) * (i / 5);
            ctx.fillStyle = '#888';
            ctx.font = '10px Microsoft YaHei';
            ctx.textAlign = 'right';
            ctx.fillText(value.toFixed(2), padding.left - 10, y + 4);
        }
    },

    drawYAxis(ctx, padding, chartHeight, yMin, yMax) {
        ctx.strokeStyle = 'rgba(0, 212, 255, 0.5)';
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.moveTo(padding.left, padding.top);
        ctx.lineTo(padding.left, padding.top + chartHeight);
        ctx.stroke();

        ctx.fillStyle = '#aaa';
        ctx.font = '11px Microsoft YaHei';
        ctx.textAlign = 'center';
        ctx.save();
        ctx.translate(20, padding.top + chartHeight / 2);
        ctx.rotate(-Math.PI / 2);
        ctx.fillText('浓度 (mg/L)', 0, 0);
        ctx.restore();
    },

    drawXAxis(ctx, padding, chartWidth, chartHeight, data) {
        ctx.strokeStyle = 'rgba(0, 212, 255, 0.5)';
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.moveTo(padding.left, padding.top + chartHeight);
        ctx.lineTo(padding.left + chartWidth, padding.top + chartHeight);
        ctx.stroke();

        ctx.fillStyle = '#aaa';
        ctx.font = '11px Microsoft YaHei';
        ctx.textAlign = 'center';
        ctx.fillText('时间', padding.left + chartWidth / 2, padding.top + chartHeight + 35);

        const labelCount = Math.min(6, data.length);
        const step = Math.max(1, Math.floor(data.length / labelCount));
        for (let i = 0; i < data.length; i += step) {
            const x = padding.left + (chartWidth / (data.length - 1)) * i;
            const time = new Date(data[i].timestamp);
            ctx.fillStyle = '#888';
            ctx.font = '10px Microsoft YaHei';
            ctx.textAlign = 'center';
            ctx.fillText(time.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }), x, padding.top + chartHeight + 15);
        }
    },

    drawTrendLine(ctx, data, padding, chartWidth, chartHeight, yMin, yMax) {
        const range = yMax - yMin;

        const lineGradient = ctx.createLinearGradient(padding.left, 0, padding.left + chartWidth, 0);
        lineGradient.addColorStop(0, '#00d4ff);
        lineGradient.addColorStop(1, '#00ff88');

        ctx.strokeStyle = lineGradient;
        ctx.lineWidth = 2;
        ctx.beginPath();

        data.forEach((point, i) => {
            const x = padding.left + (chartWidth / (data.length - 1)) * i;
            const y = padding.top + chartHeight - ((point.value - yMin) / range) * chartHeight;
            if (i === 0) {
                ctx.moveTo(x, y);
            } else {
                ctx.lineTo(x, y);
            }
        });
        ctx.stroke();

        ctx.fillStyle = 'rgba(0, 212, 255, 0.1)';
        ctx.lineTo(padding.left + chartWidth, padding.top + chartHeight);
        ctx.lineTo(padding.left, padding.top + chartHeight);
        ctx.closePath();
        ctx.fill();
    },

    drawSetpointLine(ctx, sensorInfo, padding, chartHeight, yMin, yMax) {
        if (!sensorInfo || !sensorInfo.setpoint === undefined) return;

        const range = yMax - yMin;
        const y = padding.top + chartHeight - ((sensorInfo.setpoint - yMin) / range * chartHeight;

        ctx.strokeStyle = 'rgba(241, 196, 15, 0.7)';
        ctx.setLineDash([5, 5]);
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.moveTo(padding.left, y);
        ctx.lineTo(padding.left + chartWidth, y);
        ctx.stroke();
        ctx.setLineDash([]);

        ctx.fillStyle = '#f1c40f';
        ctx.font = '10px Microsoft YaHei';
        ctx.textAlign = 'left';
        ctx.fillText(`设定值: ${sensorInfo.setpoint}`, padding.left + 10, y - 5);
    },

    drawDataPoints(ctx, data, padding, chartWidth, chartHeight, yMin, yMax) {
        const range = yMax - yMin;

        data.forEach((point, i) => {
            if (i % Math.max(1, Math.floor(data.length / 20)) !== 0) return;

            const x = padding.left + (chartWidth / (data.length - 1)) * i;
            const y = padding.top + chartHeight - ((point.value - yMin) / range) * chartHeight;

            ctx.fillStyle = '#00d4ff';
            ctx.beginPath();
            ctx.arc(x, y, 4, 0, Math.PI * 2);
            ctx.fill();

            ctx.strokeStyle = '#fff';
            ctx.lineWidth = 1;
            ctx.stroke();
        });
    },

    drawMetricsTrend(data, metricName) {
        const ctx = this.metricsCtx;
        const width = this.metricsCanvas.width;
        const height = this.metricsCanvas.height;

        ctx.clearRect(0, 0, width, height);

        const bgGradient = ctx.createLinearGradient(0, 0, 0, height);
        bgGradient.addColorStop(0, '#0f192a');
        bgGradient.addColorStop(1, '#1a2a4a');
        ctx.fillStyle = bgGradient;
        ctx.fillRect(0, 0, width, height);

        if (!data || data.length === 0) {
            ctx.fillStyle = '#888';
            ctx.font = '12px Microsoft YaHei';
            ctx.textAlign = 'center';
            ctx.fillText('加载中...', width / 2, height / 2);
            return;
        }

        const padding = { top: 20, right: 30, bottom: 30, left: 40 };
        const chartWidth = width - padding.left - padding.right;
        const chartHeight = height - padding.top - padding.bottom;

        const values = data.map(d => d.value);
        const minVal = Math.min(...values);
        const maxVal = Math.max(...values);
        const range = maxVal - minVal || 1;
        const yMin = minVal - range * 0.1;
        const yMax = maxVal + range * 0.1;
        const yRange = yMax - yMin;

        ctx.strokeStyle = 'rgba(0, 212, 255, 0.1)';
        ctx.lineWidth = 1;
        for (let i = 0; i <= 4; i++) {
            const y = padding.top + (chartHeight / 4) * i;
            ctx.beginPath();
            ctx.moveTo(padding.left, y);
            ctx.lineTo(padding.left + chartWidth, y);
            ctx.stroke();
        }

        const colors = {
            power_consumption: '#ff6b6b',
            carbon_usage: '#ffd93d',
            tn_removal_rate: '#6bcb77',
        };
        const color = colors[metricName] || '#00d4ff';

        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        ctx.beginPath();

        data.forEach((point, i) => {
            const x = padding.left + (chartWidth / (data.length - 1)) * i;
            const y = padding.top + chartHeight - ((point.value - yMin) / yRange * chartHeight;
            if (i === 0) {
                ctx.moveTo(x, y);
            } else {
                ctx.lineTo(x, y);
            }
        });
        ctx.stroke();

        ctx.fillStyle = color + '33';
        ctx.lineTo(padding.left + chartWidth, padding.top + chartHeight);
        ctx.lineTo(padding.left, padding.top + chartHeight);
        ctx.closePath();
        ctx.fill();

        ctx.fillStyle = '#aaa';
        ctx.font = '9px Microsoft YaHei';
        ctx.textAlign = 'right';
        ctx.fillText(yMax.toFixed(1), padding.left - 5, padding.top + 5);
        ctx.fillText(yMin.toFixed(1), padding.left - 5, padding.top + chartHeight + 3);
    },

    showModal(sensor, data) {
        const modal = document.getElementById('trend-modal');
        const title = document.getElementById('modal-title');
        const infoDiv = document.getElementById('trend-info');

        const sensorType = CONFIG.SENSOR_TYPES[sensor.type] || {};
        title.textContent = `${sensorType.name || sensor.id} 近6小时趋势';

        const deviation = sensor.setpoint > 0
            ? Math.abs((sensor.value - sensor.setpoint) / sensor.setpoint * 100).toFixed(1)
            : '--';

        infoDiv.innerHTML = `
            <div class="trend-info-item">
                <div class="trend-info-label">当前值</div>
                <div class="trend-info-value">${formatValue(sensor.value, sensor.type)}</div>
                <div class="trend-info-label">${sensorType.unit || ''}</div>
            </div>
            <div class="trend-info-item">
                <div class="trend-info-label">设定值</div>
                <div class="trend-info-value">${sensor.setpoint || '--'}</div>
                <div class="trend-info-label">${sensorType.unit || ''}</div>
            </div>
            <div class="trend-info-item">
                <div class="trend-info-label">偏差</div>
                <div class="trend-info-value">${deviation}%</div>
                <div class="trend-info-label">偏离设定</div>
            </div>
            <div class="trend-info-item">
                <div class="trend-info-label">更新时间</div>
                <div class="trend-info-value" style="font-size:14px">${formatTime(sensor.timestamp)}</div>
            </div>
        `;

        this.drawSensorTrend(data, sensor);
        modal.classList.add('active');
    },

    hideModal() {
        document.getElementById('trend-modal').classList.remove('active');
    }
};
