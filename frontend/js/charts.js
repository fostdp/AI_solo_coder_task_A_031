function drawLineChart(ctx, canvas, timestamps, values, options = {}) {
    const w = canvas.width;
    const h = canvas.height;
    const padding = { top: 40, right: 60, bottom: 50, left: 70 };
    const chartWidth = w - padding.left - padding.right;
    const chartHeight = h - padding.top - padding.bottom;
    const color = options.color || '#3b82f6';
    const label = options.label || '数值';
    const unit = options.unit || '';
    const showGrid = options.showGrid !== false;
    const showPoints = options.showPoints !== false;

    ctx.clearRect(0, 0, w, h);

    if (!values || values.length === 0) {
        ctx.fillStyle = '#64748b';
        ctx.font = '14px Microsoft YaHei';
        ctx.textAlign = 'center';
        ctx.fillText('暂无数据', w / 2, h / 2);
        return;
    }

    const minVal = Math.min(...values) * 0.9;
    const maxVal = Math.max(...values) * 1.1;
    const range = maxVal - minVal || 1;

    ctx.fillStyle = 'rgba(15, 23, 42, 0.95)';
    ctx.fillRect(0, 0, w, h);

    if (showGrid) {
        ctx.strokeStyle = 'rgba(100, 116, 139, 0.3)';
        ctx.lineWidth = 1;
        
        for (let i = 0; i <= 5; i++) {
            const y = padding.top + (i / 5) * chartHeight;
            ctx.beginPath();
            ctx.moveTo(padding.left, y);
            ctx.lineTo(w - padding.right, y);
            ctx.stroke();
            
            const val = maxVal - (i / 5) * range;
            ctx.fillStyle = '#94a3b8';
            ctx.font = '11px Microsoft YaHei';
            ctx.textAlign = 'right';
            ctx.fillText(val.toFixed(2), padding.left - 10, y + 4);
        }
    }

    const timeStep = chartWidth / (timestamps.length - 1 || 1);
    
    ctx.strokeStyle = color;
    ctx.lineWidth = 3;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    
    const gradient = ctx.createLinearGradient(0, padding.top, 0, h - padding.bottom);
    gradient.addColorStop(0, color + '60');
    gradient.addColorStop(1, color + '00');
    
    ctx.beginPath();
    ctx.moveTo(padding.left, h - padding.bottom);
    for (let i = 0; i < values.length; i++) {
        const x = padding.left + i * timeStep;
        const y = padding.top + ((maxVal - values[i]) / range) * chartHeight;
        if (i === 0) {
            ctx.lineTo(x, y);
        } else {
            const prevX = padding.left + (i - 1) * timeStep;
            const prevY = padding.top + ((maxVal - values[i - 1]) / range) * chartHeight;
            const cpX = (prevX + x) / 2;
            ctx.quadraticCurveTo(prevX, prevY, cpX, (prevY + y) / 2);
            ctx.quadraticCurveTo(cpX, (prevY + y) / 2, x, y);
        }
    }
    ctx.lineTo(padding.left + (values.length - 1) * timeStep, h - padding.bottom);
    ctx.closePath();
    ctx.fillStyle = gradient;
    ctx.fill();

    ctx.beginPath();
    for (let i = 0; i < values.length; i++) {
        const x = padding.left + i * timeStep;
        const y = padding.top + ((maxVal - values[i]) / range) * chartHeight;
        if (i === 0) {
            ctx.moveTo(x, y);
        } else {
            const prevX = padding.left + (i - 1) * timeStep;
            const prevY = padding.top + ((maxVal - values[i - 1]) / range) * chartHeight;
            const cpX = (prevX + x) / 2;
            ctx.quadraticCurveTo(prevX, prevY, cpX, (prevY + y) / 2);
            ctx.quadraticCurveTo(cpX, (prevY + y) / 2, x, y);
        }
    }
    ctx.stroke();

    if (showPoints) {
        for (let i = 0; i < values.length; i++) {
            const x = padding.left + i * timeStep;
            const y = padding.top + ((maxVal - values[i]) / range) * chartHeight;
            
            if (i % Math.max(1, Math.floor(values.length / 10)) === 0 || i === values.length - 1) {
                ctx.fillStyle = '#fff';
                ctx.strokeStyle = color;
                ctx.lineWidth = 2;
                ctx.beginPath();
                ctx.arc(x, y, 5, 0, Math.PI * 2);
                ctx.fill();
                ctx.stroke();
            }
        }
    }

    const timeLabels = [];
    for (let i = 0; i < timestamps.length; i++) {
        if (i % Math.max(1, Math.floor(timestamps.length / 8)) === 0 || i === timestamps.length - 1) {
            timeLabels.push({ index: i, time: timestamps[i] });
        }
    }
    
    ctx.fillStyle = '#94a3b8';
    ctx.font = '10px Microsoft YaHei';
    ctx.textAlign = 'center';
    for (const tl of timeLabels) {
        const x = padding.left + tl.index * timeStep;
        const timeStr = new Date(tl.time).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
        ctx.fillText(timeStr, x, h - padding.bottom + 20);
    }

    ctx.fillStyle = '#e4e4e7';
    ctx.font = 'bold 14px Microsoft YaHei';
    ctx.textAlign = 'center';
    ctx.fillText(`${label} 趋势曲线 (${unit})`, w / 2, 25);

    ctx.fillStyle = '#64748b';
    ctx.font = '11px Microsoft YaHei';
    ctx.textAlign = 'right';
    ctx.fillText(`最新值: ${values[values.length - 1].toFixed(2)} ${unit}`, w - padding.right, 20);
    
    const avgVal = values.reduce((a, b) => a + b, 0) / values.length;
    ctx.fillText(`平均值: ${avgVal.toFixed(2)} ${unit}`, w - padding.right, 38);
}


class KPIDashboard {
    constructor(containers) {
        this.containers = containers;
        this.data = {
            energy: { current: 0, target: 0, history: [], labels: [] },
            carbon: { current: 0, target: 0, history: [], labels: [] },
            removal: { current: 0, target: 0, history: [], labels: [] }
        };
    }

    update(type, value, target) {
        if (!this.data[type]) return;
        
        this.data[type].current = value;
        this.data[type].target = target;
        
        const now = new Date();
        this.data[type].history.push(value);
        this.data[type].labels.push(now);
        
        if (this.data[type].history.length > 24) {
            this.data[type].history.shift();
            this.data[type].labels.shift();
        }
        
        this.render(type);
    }

    render(type) {
        const container = this.containers[type];
        if (!container) return;
        
        const canvas = container.querySelector('.kpi-sparkline');
        const valueEl = container.querySelector('.kpi-value');
        const trendEl = container.querySelector('.kpi-trend');
        
        if (canvas) {
            const ctx = canvas.getContext('2d');
            canvas.width = canvas.parentElement.clientWidth;
            canvas.height = 60;
            this.drawSparkline(ctx, canvas, this.data[type].history, type);
        }
        
        if (valueEl) {
            valueEl.textContent = this.data[type].current.toFixed(3);
        }
        
        if (trendEl && this.data[type].history.length >= 2) {
            const hist = this.data[type].history;
            const diff = hist[hist.length - 1] - hist[hist.length - 2];
            const percent = this.data[type].target > 0 
                ? ((this.data[type].current / this.data[type].target - 1) * 100) 
                : 0;
            
            trendEl.textContent = `${percent >= 0 ? '+' : ''}${percent.toFixed(1)}%`;
            trendEl.className = 'kpi-trend ' + (percent > 10 ? 'up' : percent < -10 ? 'down' : 'stable');
        }
    }

    drawSparkline(ctx, canvas, values, type) {
        const w = canvas.width;
        const h = canvas.height;
        
        ctx.clearRect(0, 0, w, h);
        
        if (values.length < 2) return;
        
        const minVal = Math.min(...values) * 0.95;
        const maxVal = Math.max(...values) * 1.05;
        const range = maxVal - minVal || 1;
        
        const colors = {
            energy: '#ef4444',
            carbon: '#f59e0b',
            removal: '#10b981'
        };
        
        const color = colors[type] || '#3b82f6';
        
        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        ctx.lineCap = 'round';
        ctx.lineJoin = 'round';
        
        const gradient = ctx.createLinearGradient(0, 0, 0, h);
        gradient.addColorStop(0, color + '40');
        gradient.addColorStop(1, color + '00');
        
        const step = w / (values.length - 1);
        
        ctx.beginPath();
        ctx.moveTo(0, h);
        for (let i = 0; i < values.length; i++) {
            const x = i * step;
            const y = h - ((values[i] - minVal) / range) * (h - 10);
            ctx.lineTo(x, y);
        }
        ctx.lineTo(w, h);
        ctx.closePath();
        ctx.fillStyle = gradient;
        ctx.fill();
        
        ctx.beginPath();
        for (let i = 0; i < values.length; i++) {
            const x = i * step;
            const y = h - ((values[i] - minVal) / range) * (h - 10);
            if (i === 0) {
                ctx.moveTo(x, y);
            } else {
                ctx.lineTo(x, y);
            }
        }
        ctx.stroke();
        
        const lastY = h - ((values[values.length - 1] - minVal) / range) * (h - 10);
        ctx.fillStyle = '#fff';
        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.arc(w - 5, lastY, 4, 0, Math.PI * 2);
        ctx.fill();
        ctx.stroke();
    }
}


function drawGauge(ctx, canvas, value, min, max, target, options = {}) {
    const w = canvas.width;
    const h = canvas.height;
    const centerX = w / 2;
    const centerY = h / 2;
    const radius = Math.min(w, h) / 2 - 20;
    const color = options.color || '#3b82f6';
    const label = options.label || '';
    const unit = options.unit || '';

    ctx.clearRect(0, 0, w, h);

    const startAngle = Math.PI * 0.75;
    const endAngle = Math.PI * 2.25;
    const totalAngle = endAngle - startAngle;

    const bgGradient = ctx.createLinearGradient(centerX, centerY - radius, centerX, centerY + radius);
    bgGradient.addColorStop(0, 'rgba(30, 41, 59, 0.8)');
    bgGradient.addColorStop(1, 'rgba(15, 23, 42, 0.9)');
    
    ctx.fillStyle = bgGradient;
    ctx.beginPath();
    ctx.arc(centerX, centerY, radius + 10, 0, Math.PI * 2);
    ctx.fill();

    ctx.strokeStyle = 'rgba(100, 116, 139, 0.3)';
    ctx.lineWidth = 12;
    ctx.beginPath();
    ctx.arc(centerX, centerY, radius, startAngle, endAngle);
    ctx.stroke();

    const percent = Math.max(0, Math.min(1, (value - min) / (max - min)));
    const valueAngle = startAngle + percent * totalAngle;

    const gaugeGradient = ctx.createLinearGradient(centerX - radius, centerY, centerX + radius, centerY);
    if (percent < 0.4) {
        gaugeGradient.addColorStop(0, '#10b981');
        gaugeGradient.addColorStop(1, '#22c55e');
    } else if (percent < 0.7) {
        gaugeGradient.addColorStop(0, '#f59e0b');
        gaugeGradient.addColorStop(1, '#fbbf24');
    } else {
        gaugeGradient.addColorStop(0, '#ef4444');
        gaugeGradient.addColorStop(1, '#f87171');
    }

    ctx.strokeStyle = gaugeGradient;
    ctx.lineWidth = 12;
    ctx.lineCap = 'round';
    ctx.beginPath();
    ctx.arc(centerX, centerY, radius, startAngle, valueAngle);
    ctx.stroke();

    const targetPercent = (target - min) / (max - min);
    const targetAngle = startAngle + targetPercent * totalAngle;
    const tx1 = centerX + Math.cos(targetAngle) * (radius - 20);
    const ty1 = centerY + Math.sin(targetAngle) * (radius - 20);
    const tx2 = centerX + Math.cos(targetAngle) * (radius + 20);
    const ty2 = centerY + Math.sin(targetAngle) * (radius + 20);
    
    ctx.strokeStyle = '#ffffff';
    ctx.lineWidth = 2;
    ctx.setLineDash([5, 5]);
    ctx.beginPath();
    ctx.moveTo(tx1, ty1);
    ctx.lineTo(tx2, ty2);
    ctx.stroke();
    ctx.setLineDash([]);

    ctx.fillStyle = '#e4e4e7';
    ctx.font = 'bold 28px Microsoft YaHei';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(value.toFixed(2), centerX, centerY - 10);

    ctx.fillStyle = '#94a3b8';
    ctx.font = '12px Microsoft YaHei';
    ctx.fillText(unit, centerX, centerY + 20);

    ctx.fillStyle = '#cbd5e1';
    ctx.font = 'bold 13px Microsoft YaHei';
    ctx.fillText(label, centerX, centerY + 40);

    ctx.fillStyle = '#64748b';
    ctx.font = '10px Microsoft YaHei';
    ctx.textAlign = 'left';
    ctx.fillText(min.toFixed(1), centerX - radius + 10, centerY + radius + 5);
    ctx.textAlign = 'right';
    ctx.fillText(max.toFixed(1), centerX + radius - 10, centerY + radius + 5);
}


function drawBarChart(ctx, canvas, labels, values, options = {}) {
    const w = canvas.width;
    const h = canvas.height;
    const padding = { top: 40, right: 30, bottom: 60, left: 60 };
    const chartWidth = w - padding.left - padding.right;
    const chartHeight = h - padding.top - padding.bottom;
    const colors = options.colors || ['#3b82f6', '#8b5cf6', '#10b981', '#f59e0b', '#ef4444'];
    const title = options.title || '';

    ctx.clearRect(0, 0, w, h);

    ctx.fillStyle = 'rgba(15, 23, 42, 0.95)';
    ctx.fillRect(0, 0, w, h);

    const maxVal = Math.max(...values) * 1.2;

    ctx.strokeStyle = 'rgba(100, 116, 139, 0.3)';
    ctx.lineWidth = 1;
    for (let i = 0; i <= 4; i++) {
        const y = padding.top + (i / 4) * chartHeight;
        ctx.beginPath();
        ctx.moveTo(padding.left, y);
        ctx.lineTo(w - padding.right, y);
        ctx.stroke();

        const val = maxVal - (i / 4) * maxVal;
        ctx.fillStyle = '#94a3b8';
        ctx.font = '11px Microsoft YaHei';
        ctx.textAlign = 'right';
        ctx.fillText(val.toFixed(0), padding.left - 10, y + 4);
    }

    const barWidth = (chartWidth / labels.length) * 0.6;
    const gap = (chartWidth / labels.length) * 0.4;

    for (let i = 0; i < labels.length; i++) {
        const x = padding.left + i * (barWidth + gap) + gap / 2;
        const barHeight = (values[i] / maxVal) * chartHeight;
        const y = padding.top + chartHeight - barHeight;

        const gradient = ctx.createLinearGradient(x, y, x, y + barHeight);
        gradient.addColorStop(0, colors[i % colors.length]);
        gradient.addColorStop(1, colors[i % colors.length] + '80');

        ctx.fillStyle = gradient;
        ctx.beginPath();
        ctx.roundRect(x, y, barWidth, barHeight, 6);
        ctx.fill();

        ctx.fillStyle = '#e4e4e7';
        ctx.font = 'bold 12px Microsoft YaHei';
        ctx.textAlign = 'center';
        ctx.fillText(values[i].toFixed(1), x + barWidth / 2, y - 8);

        ctx.fillStyle = '#94a3b8';
        ctx.font = '11px Microsoft YaHei';
        ctx.fillText(labels[i], x + barWidth / 2, h - padding.bottom + 20);
    }

    if (title) {
        ctx.fillStyle = '#e4e4e7';
        ctx.font = 'bold 14px Microsoft YaHei';
        ctx.textAlign = 'center';
        ctx.fillText(title, w / 2, 25);
    }
}


function drawDonutChart(ctx, canvas, data, options = {}) {
    const w = canvas.width;
    const h = canvas.height;
    const centerX = w / 2;
    const centerY = h / 2;
    const radius = Math.min(w, h) / 2 - 30;
    const innerRadius = radius * 0.6;
    const title = options.title || '';
    const total = data.reduce((sum, item) => sum + item.value, 0);

    ctx.clearRect(0, 0, w, h);

    ctx.fillStyle = 'rgba(15, 23, 42, 0.95)';
    ctx.fillRect(0, 0, w, h);

    let startAngle = -Math.PI / 2;

    for (const item of data) {
        const sliceAngle = (item.value / total) * Math.PI * 2;
        const endAngle = startAngle + sliceAngle;

        const gradient = ctx.createRadialGradient(centerX, centerY, innerRadius, centerX, centerY, radius);
        gradient.addColorStop(0, item.color);
        gradient.addColorStop(1, item.color + 'cc');

        ctx.fillStyle = gradient;
        ctx.beginPath();
        ctx.moveTo(centerX + Math.cos(startAngle) * innerRadius, centerY + Math.sin(startAngle) * innerRadius);
        ctx.arc(centerX, centerY, innerRadius, startAngle, endAngle);
        ctx.arc(centerX, centerY, radius, endAngle, startAngle, true);
        ctx.closePath();
        ctx.fill();

        const midAngle = startAngle + sliceAngle / 2;
        const labelX = centerX + Math.cos(midAngle) * (radius + innerRadius) / 2;
        const labelY = centerY + Math.sin(midAngle) * (radius + innerRadius) / 2;

        ctx.fillStyle = '#fff';
        ctx.font = 'bold 12px Microsoft YaHei';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        const percent = ((item.value / total) * 100).toFixed(1);
        ctx.fillText(`${percent}%`, labelX, labelY);

        startAngle = endAngle;
    }

    ctx.fillStyle = '#e4e4e7';
    ctx.font = 'bold 24px Microsoft YaHei';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(total.toFixed(0), centerX, centerY - 5);

    ctx.fillStyle = '#94a3b8';
    ctx.font = '11px Microsoft YaHei';
    ctx.fillText('合计', centerX, centerY + 15);

    if (title) {
        ctx.fillStyle = '#e4e4e7';
        ctx.font = 'bold 14px Microsoft YaHei';
        ctx.fillText(title, centerX, 25);
    }

    let legendY = centerY + radius + 20;
    for (let i = 0; i < data.length; i++) {
        const item = data[i];
        const legendX = w / 2 - data.length * 50 + i * 100;
        
        ctx.fillStyle = item.color;
        ctx.fillRect(legendX - 20, legendY, 12, 12);
        
        ctx.fillStyle = '#94a3b8';
        ctx.font = '11px Microsoft YaHei';
        ctx.textAlign = 'left';
        ctx.fillText(item.label, legendX, legendY + 10);
    }
}
