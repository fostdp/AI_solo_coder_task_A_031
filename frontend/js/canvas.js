class ProcessLayout {
    constructor(canvasId) {
        this.canvas = document.getElementById(canvasId);
        this.ctx = this.canvas.getContext('2d');
        this.sensors = [];
        this.sections = CONFIG.PROCESS_SECTIONS;
        this.zoom = 1;
        this.offsetX = 0;
        this.offsetY = 0;
        this.selectedSensor = null;
        this.filter = 'all';
        this.sensorValues = {};
        this.hoveredSensor = null;
        this.dragging = false;
        this.lastMouseX = 0;
        this.lastMouseY = 0;
        
        this.init();
    }
    
    init() {
        this.resize();
        this.setupEventListeners();
        this.loadSensors();
        this.animate();
    }
    
    resize() {
        const container = this.canvas.parentElement;
        this.canvas.width = container.clientWidth;
        this.canvas.height = container.clientHeight;
        this.draw();
    }
    
    setupEventListeners() {
        window.addEventListener('resize', () => this.resize());
        
        this.canvas.addEventListener('click', (e) => this.handleClick(e));
        this.canvas.addEventListener('mousemove', (e) => this.handleMouseMove(e));
        this.canvas.addEventListener('mousedown', (e) => this.handleMouseDown(e));
        this.canvas.addEventListener('mouseup', (e) => this.handleMouseUp(e));
        this.canvas.addEventListener('mouseleave', (e) => this.handleMouseUp(e));
        
        document.getElementById('zoomIn')?.addEventListener('click', () => {
            this.zoom = Math.min(this.zoom * 1.2, 3);
            this.draw();
        });
        
        document.getElementById('zoomOut')?.addEventListener('click', () => {
            this.zoom = Math.max(this.zoom / 1.2, 0.5);
            this.draw();
        });
        
        document.getElementById('resetView')?.addEventListener('click', () => {
            this.zoom = 1;
            this.offsetX = 0;
            this.offsetY = 0;
            this.draw();
        });
        
        document.getElementById('sensorFilter')?.addEventListener('change', (e) => {
            this.filter = e.target.value;
            this.draw();
        });
    }
    
    async loadSensors() {
        try {
            const response = await fetch(`${CONFIG.API_BASE_URL}/sensors`);
            this.sensors = await response.json();
            this.draw();
        } catch (error) {
            console.error('Failed to load sensors:', error);
            this.generateMockSensors();
        }
    }
    
    generateMockSensors() {
        const types = ['DO', 'NH3', 'NO3', 'PO4'];
        const counts = { DO: 30, NH3: 20, NO3: 15, PO4: 10 };
        const locations = ['anaerobic', 'anoxic', 'aerobic1', 'aerobic2', 'aerobic3', 'effluent'];
        
        this.sensors = [];
        let id = 0;
        
        for (const type of types) {
            for (let i = 0; i < counts[type]; i++) {
                const location = locations[Math.floor(i / (counts[type] / locations.length)) % locations.length];
                const section = this.sections.find(s => s.id === location) || this.sections[5];
                
                this.sensors.push({
                    sensor_id: `${type}-${String(i + 1).padStart(3, '0')}`,
                    type: type,
                    location: location,
                    setpoint: CONFIG.SENSOR_TYPES[type].setpoint,
                    x: section.x + section.width / 2 + (Math.random() - 0.5) * section.width * 0.6,
                    y: section.y + section.height / 2 + (Math.random() - 0.5) * section.height * 0.6,
                    description: CONFIG.SENSOR_TYPES[type].name + '传感器'
                });
                
                this.sensorValues[`${type}-${String(i + 1).padStart(3, '0')}`] = {
                    value: CONFIG.SENSOR_TYPES[type].setpoint * (0.9 + Math.random() * 0.2),
                    timestamp: new Date()
                };
            }
        }
        
        this.draw();
    }
    
    updateSensorValue(sensorId, value, timestamp) {
        this.sensorValues[sensorId] = { value, timestamp: new Date(timestamp) };
        this.draw();
    }
    
    handleClick(e) {
        const rect = this.canvas.getBoundingClientRect();
        const x = (e.clientX - rect.left - this.offsetX) / this.zoom;
        const y = (e.clientY - rect.top - this.offsetY) / this.zoom;
        
        for (const sensor of this.sensors) {
            if (this.filter !== 'all' && sensor.type !== this.filter) continue;
            
            const dx = x - sensor.x;
            const dy = y - sensor.y;
            if (Math.sqrt(dx * dx + dy * dy) < 12 / this.zoom) {
                this.selectedSensor = sensor;
                this.showSensorModal(sensor);
                return;
            }
        }
    }
    
    handleMouseMove(e) {
        const rect = this.canvas.getBoundingClientRect();
        const x = (e.clientX - rect.left - this.offsetX) / this.zoom;
        const y = (e.clientY - rect.top - this.offsetY) / this.zoom;
        
        if (this.dragging) {
            this.offsetX += e.clientX - this.lastMouseX;
            this.offsetY += e.clientY - this.lastMouseY;
            this.lastMouseX = e.clientX;
            this.lastMouseY = e.clientY;
            this.draw();
            return;
        }
        
        this.hoveredSensor = null;
        for (const sensor of this.sensors) {
            if (this.filter !== 'all' && sensor.type !== this.filter) continue;
            
            const dx = x - sensor.x;
            const dy = y - sensor.y;
            if (Math.sqrt(dx * dx + dy * dy) < 12 / this.zoom) {
                this.hoveredSensor = sensor;
                this.canvas.style.cursor = 'pointer';
                break;
            }
        }
        
        if (!this.hoveredSensor) {
            this.canvas.style.cursor = this.dragging ? 'grabbing' : 'grab';
        }
        
        this.draw();
    }
    
    handleMouseDown(e) {
        this.dragging = true;
        this.lastMouseX = e.clientX;
        this.lastMouseY = e.clientY;
        this.canvas.style.cursor = 'grabbing';
    }
    
    handleMouseUp(e) {
        this.dragging = false;
        this.canvas.style.cursor = 'grab';
    }
    
    async showSensorModal(sensor) {
        const modal = document.getElementById('sensorModal');
        const valueData = this.sensorValues[sensor.sensor_id] || { value: sensor.setpoint, timestamp: new Date() };
        
        document.getElementById('modalTitle').textContent = `${CONFIG.SENSOR_TYPES[sensor.type].name}传感器详情`;
        document.getElementById('modalSensorId').textContent = sensor.sensor_id;
        document.getElementById('modalSensorType').textContent = CONFIG.SENSOR_TYPES[sensor.type].name;
        document.getElementById('modalSensorLocation').textContent = CONFIG.LOCATION_NAMES[sensor.location] || sensor.location;
        document.getElementById('modalSensorValue').textContent = `${valueData.value.toFixed(3)} ${CONFIG.SENSOR_TYPES[sensor.type].unit}`;
        document.getElementById('modalSensorSetpoint').textContent = `${sensor.setpoint} ${CONFIG.SENSOR_TYPES[sensor.type].unit}`;
        
        const deviation = Math.abs((valueData.value - sensor.setpoint) / sensor.setpoint * 100);
        const deviationSpan = document.getElementById('modalSensorDeviation');
        deviationSpan.textContent = `${deviation.toFixed(1)}%`;
        deviationSpan.className = 'info-value ' + this.getDeviationClass(deviation);
        
        const statusSpan = document.getElementById('modalSensorStatus');
        const level = this.getDeviationLevel(deviation);
        statusSpan.textContent = level.name;
        statusSpan.className = 'info-value ' + this.getDeviationClass(deviation);
        
        modal.classList.add('active');
        
        await drawSensorTrend(sensor.sensor_id);
    }
    
    getDeviationLevel(deviation) {
        if (deviation < CONFIG.DEVIATION_LEVELS.GREEN.max) return CONFIG.DEVIATION_LEVELS.GREEN;
        if (deviation < CONFIG.DEVIATION_LEVELS.YELLOW.max) return CONFIG.DEVIATION_LEVELS.YELLOW;
        return CONFIG.DEVIATION_LEVELS.RED;
    }
    
    getDeviationClass(deviation) {
        if (deviation < CONFIG.DEVIATION_LEVELS.GREEN.max) return 'green';
        if (deviation < CONFIG.DEVIATION_LEVELS.YELLOW.max) return 'yellow';
        return 'red';
    }
    
    getSensorColor(sensor) {
        const valueData = this.sensorValues[sensor.sensor_id];
        if (!valueData) return '#6b7280';
        
        const now = new Date();
        if (now - valueData.timestamp > 5 * 60 * 1000) {
            return '#6b7280';
        }
        
        const deviation = Math.abs((valueData.value - sensor.setpoint) / sensor.setpoint * 100);
        return this.getDeviationLevel(deviation).color;
    }
    
    draw() {
        const ctx = this.ctx;
        const w = this.canvas.width;
        const h = this.canvas.height;
        
        ctx.clearRect(0, 0, w, h);
        
        ctx.save();
        ctx.translate(this.offsetX, this.offsetY);
        ctx.scale(this.zoom, this.zoom);
        
        this.drawGrid();
        this.drawWaterFlow();
        this.drawSections();
        this.drawPipes();
        this.drawSensors();
        
        if (this.hoveredSensor) {
            this.drawSensorTooltip(this.hoveredSensor);
        }
        
        ctx.restore();
    }
    
    drawGrid() {
        const ctx = this.ctx;
        const gridSize = 50;
        
        ctx.strokeStyle = 'rgba(59, 130, 246, 0.05)';
        ctx.lineWidth = 1;
        
        for (let x = 0; x < 1200; x += gridSize) {
            ctx.beginPath();
            ctx.moveTo(x, 0);
            ctx.lineTo(x, 800);
            ctx.stroke();
        }
        
        for (let y = 0; y < 800; y += gridSize) {
            ctx.beginPath();
            ctx.moveTo(0, y);
            ctx.lineTo(1200, y);
            ctx.stroke();
        }
    }
    
    drawSections() {
        const ctx = this.ctx;
        
        for (const section of this.sections) {
            const gradient = ctx.createLinearGradient(section.x, section.y, section.x, section.y + section.height);
            gradient.addColorStop(0, 'rgba(30, 41, 59, 0.9)');
            gradient.addColorStop(1, 'rgba(15, 23, 42, 0.9)');
            
            ctx.fillStyle = gradient;
            ctx.strokeStyle = '#475569';
            ctx.lineWidth = 2;
            
            this.roundRect(ctx, section.x, section.y, section.width, section.height, 8);
            ctx.fill();
            ctx.stroke();
            
            ctx.fillStyle = '#cbd5e1';
            ctx.font = 'bold 12px Microsoft YaHei';
            ctx.textAlign = 'center';
            ctx.fillText(section.name, section.x + section.width / 2, section.y + 20);
            
            const typeColors = {
                pre_treatment: '#06b6d4',
                primary_treatment: '#8b5cf6',
                biological: '#10b981',
                secondary_treatment: '#f59e0b',
                advanced_treatment: '#ef4444'
            };
            
            ctx.fillStyle = typeColors[section.type] || '#64748b';
            ctx.font = '10px Microsoft YaHei';
            ctx.fillText(this.getSectionTypeName(section.type), section.x + section.width / 2, section.y + 35);
            
            ctx.fillStyle = 'rgba(59, 130, 246, 0.3)';
            ctx.fillRect(section.x + 5, section.y + section.height - 15, section.width - 10, 8);
        }
    }
    
    getSectionTypeName(type) {
        const names = {
            pre_treatment: '预处理',
            primary_treatment: '一级处理',
            biological: '生化处理',
            secondary_treatment: '二级处理',
            advanced_treatment: '深度处理'
        };
        return names[type] || type;
    }
    
    drawWaterFlow() {
        const ctx = this.ctx;
        
        ctx.strokeStyle = 'rgba(59, 130, 246, 0.6)';
        ctx.lineWidth = 4;
        ctx.lineCap = 'round';
        
        const points = [
            { x: 10, y: 300 },
            { x: 50, y: 300 },
            { x: 90, y: 300 },
            { x: 190, y: 300 },
            { x: 290, y: 300 },
            { x: 400, y: 300 },
            { x: 400, y: 380 },
            { x: 160, y: 380 },
            { x: 160, y: 475 },
            { x: 300, y: 475 },
            { x: 440, y: 475 },
            { x: 600, y: 475 },
            { x: 760, y: 475 },
            { x: 920, y: 475 },
            { x: 1050, y: 475 },
            { x: 1150, y: 475 }
        ];
        
        ctx.beginPath();
        ctx.moveTo(points[0].x, points[0].y);
        for (let i = 1; i < points.length; i++) {
            ctx.lineTo(points[i].x, points[i].y);
        }
        ctx.stroke();
        
        const time = Date.now() / 1000;
        for (let i = 0; i < 8; i++) {
            const t = ((time * 0.3 + i * 0.125) % 1);
            const idx = Math.floor(t * (points.length - 1));
            const frac = (t * (points.length - 1)) % 1;
            
            if (idx < points.length - 1) {
                const x = points[idx].x + (points[idx + 1].x - points[idx].x) * frac;
                const y = points[idx].y + (points[idx + 1].y - points[idx].y) * frac;
                
                ctx.fillStyle = `rgba(96, 165, 250, ${0.8 - t * 0.5})`;
                ctx.beginPath();
                ctx.arc(x, y, 5, 0, Math.PI * 2);
                ctx.fill();
            }
        }
    }
    
    drawPipes() {
        const ctx = this.ctx;
        
        ctx.strokeStyle = 'rgba(148, 163, 184, 0.4)';
        ctx.lineWidth = 8;
        ctx.lineCap = 'round';
        
        ctx.beginPath();
        ctx.moveTo(920, 400);
        ctx.lineTo(920, 320);
        ctx.lineTo(420, 320);
        ctx.lineTo(420, 400);
        ctx.stroke();
        
        ctx.fillStyle = 'rgba(148, 163, 184, 0.6)';
        ctx.font = '10px Microsoft YaHei';
        ctx.textAlign = 'center';
        ctx.fillText('硝化液回流 (300%)', 650, 315);
        
        ctx.strokeStyle = 'rgba(139, 92, 246, 0.4)';
        ctx.lineWidth = 6;
        
        ctx.beginPath();
        ctx.moveTo(920, 550);
        ctx.lineTo(920, 600);
        ctx.lineTo(200, 600);
        ctx.lineTo(200, 550);
        ctx.stroke();
        
        ctx.fillStyle = 'rgba(139, 92, 246, 0.6)';
        ctx.fillText('污泥回流 (100%)', 550, 595);
    }
    
    drawSensors() {
        const ctx = this.ctx;
        
        for (const sensor of this.sensors) {
            if (this.filter !== 'all' && sensor.type !== this.filter) continue;
            
            const color = this.getSensorColor(sensor);
            const isHovered = this.hoveredSensor?.sensor_id === sensor.sensor_id;
            const isSelected = this.selectedSensor?.sensor_id === sensor.sensor_id;
            const radius = isHovered || isSelected ? 10 : 8;
            
            const gradient = ctx.createRadialGradient(sensor.x, sensor.y, 0, sensor.x, sensor.y, radius * 2);
            gradient.addColorStop(0, color);
            gradient.addColorStop(0.5, color + '80');
            gradient.addColorStop(1, 'transparent');
            
            ctx.fillStyle = gradient;
            ctx.beginPath();
            ctx.arc(sensor.x, sensor.y, radius * 2, 0, Math.PI * 2);
            ctx.fill();
            
            ctx.fillStyle = color;
            ctx.strokeStyle = '#fff';
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.arc(sensor.x, sensor.y, radius, 0, Math.PI * 2);
            ctx.fill();
            ctx.stroke();
            
            if (isHovered || isSelected) {
                ctx.fillStyle = CONFIG.SENSOR_TYPES[sensor.type].color;
                ctx.font = 'bold 10px Microsoft YaHei';
                ctx.textAlign = 'center';
                ctx.fillText(sensor.sensor_id, sensor.x, sensor.y - radius - 5);
            }
        }
    }
    
    drawSensorTooltip(sensor) {
        const ctx = this.ctx;
        const valueData = this.sensorValues[sensor.sensor_id] || { value: sensor.setpoint, timestamp: new Date() };
        
        const padding = 10;
        const text = `${sensor.sensor_id}: ${valueData.value.toFixed(2)} ${CONFIG.SENSOR_TYPES[sensor.type].unit}`;
        
        ctx.font = '12px Microsoft YaHei';
        const textWidth = ctx.measureText(text).width;
        const width = textWidth + padding * 2;
        const height = 30;
        let x = sensor.x + 15;
        let y = sensor.y - height / 2;
        
        if (x + width > 1150) x = sensor.x - width - 15;
        if (y < 0) y = 5;
        if (y + height > 750) y = 750 - height;
        
        ctx.fillStyle = 'rgba(15, 23, 42, 0.95)';
        ctx.strokeStyle = CONFIG.SENSOR_TYPES[sensor.type].color;
        ctx.lineWidth = 2;
        this.roundRect(ctx, x, y, width, height, 6);
        ctx.fill();
        ctx.stroke();
        
        ctx.fillStyle = '#e4e4e7';
        ctx.textAlign = 'left';
        ctx.fillText(text, x + padding, y + 20);
    }
    
    roundRect(ctx, x, y, width, height, radius) {
        ctx.beginPath();
        ctx.moveTo(x + radius, y);
        ctx.lineTo(x + width - radius, y);
        ctx.quadraticCurveTo(x + width, y, x + width, y + radius);
        ctx.lineTo(x + width, y + height - radius);
        ctx.quadraticCurveTo(x + width, y + height, x + width - radius, y + height);
        ctx.lineTo(x + radius, y + height);
        ctx.quadraticCurveTo(x, y + height, x, y + height - radius);
        ctx.lineTo(x, y + radius);
        ctx.quadraticCurveTo(x, y, x + radius, y);
        ctx.closePath();
    }
    
    animate() {
        this.draw();
        requestAnimationFrame(() => this.animate());
    }
}


class BiologicalProfile {
    constructor(canvasId) {
        this.canvas = document.getElementById(canvasId);
        this.ctx = this.canvas.getContext('2d');
        this.zone = 'all';
        this.profileData = [];
        this.init();
    }
    
    init() {
        this.resize();
        this.setupEventListeners();
        this.loadProfileData();
        this.animate();
    }
    
    resize() {
        const container = this.canvas.parentElement;
        this.canvas.width = container.clientWidth;
        this.canvas.height = container.clientHeight;
        this.draw();
    }
    
    setupEventListeners() {
        window.addEventListener('resize', () => this.resize());
        
        document.getElementById('profileZone')?.addEventListener('change', (e) => {
            this.zone = e.target.value;
            this.loadProfileData();
        });
    }
    
    async loadProfileData() {
        try {
            const response = await fetch(`${CONFIG.API_BASE_URL}/biological-profile`);
            this.profileData = await response.json();
        } catch (error) {
            console.error('Failed to load profile data:', error);
            this.generateMockProfile();
        }
        this.draw();
    }
    
    generateMockProfile() {
        this.profileData = [];
        const depths = [0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5.0];
        const zones = this.zone === 'all' ? CONFIG.BIOLOGICAL_ZONES : [this.zone];
        
        for (const zone of zones) {
            for (const depth of depths) {
                const baseDO = zone === 'anaerobic' ? 0.2 :
                              zone === 'anoxic' ? 0.5 :
                              zone === 'aerobic1' ? 2.5 :
                              zone === 'aerobic2' ? 2.0 : 1.5;
                
                const doValue = Math.max(0, baseDO - 0.1 * (depth / 5.0) + (Math.random() - 0.5) * 0.2);
                
                this.profileData.push({
                    zone: zone,
                    depth: depth,
                    do: doValue,
                    color: this.getDOColor(doValue)
                });
            }
        }
    }
    
    getDOColor(do) {
        if (do < 0.5) return '#2c3e50';
        if (do < 1.5) return '#e74c3c';
        if (do < 2.0) return '#f39c12';
        if (do < 3.0) return '#27ae60';
        return '#3498db';
    }
    
    draw() {
        const ctx = this.ctx;
        const w = this.canvas.width;
        const h = this.canvas.height;
        
        ctx.clearRect(0, 0, w, h);
        
        const zones = this.zone === 'all' ? CONFIG.BIOLOGICAL_ZONES : [this.zone];
        const zoneWidth = (w - 150) / zones.length;
        const depthHeight = h - 120;
        const maxDepth = 5.0;
        
        this.drawAxes(ctx, w, h, zoneWidth, depthHeight, maxDepth, zones);
        
        for (let zi = 0; zi < zones.length; zi++) {
            const zone = zones[zi];
            const zoneData = this.profileData.filter(d => d.zone === zone);
            const x = 100 + zi * zoneWidth;
            
            this.drawZoneTank(ctx, x, 60, zoneWidth - 20, depthHeight, zone);
            
            for (let i = 0; i < zoneData.length - 1; i++) {
                const d1 = zoneData[i];
                const d2 = zoneData[i + 1];
                
                const y1 = 60 + (d1.depth / maxDepth) * depthHeight;
                const y2 = 60 + (d2.depth / maxDepth) * depthHeight;
                
                const gradient = ctx.createLinearGradient(x, y1, x + zoneWidth - 20, y1);
                gradient.addColorStop(0, d1.color);
                gradient.addColorStop(1, d2.color);
                
                ctx.fillStyle = gradient;
                ctx.fillRect(x, y1, zoneWidth - 20, y2 - y1);
            }
            
            for (const d of zoneData) {
                const y = 60 + (d.depth / maxDepth) * depthHeight;
                
                ctx.fillStyle = '#fff';
                ctx.strokeStyle = d1?.color || '#333';
                ctx.lineWidth = 2;
                ctx.beginPath();
                ctx.arc(x + (zoneWidth - 20) / 2, y, 6, 0, Math.PI * 2);
                ctx.fill();
                ctx.stroke();
                
                ctx.fillStyle = '#fff';
                ctx.font = '10px Microsoft YaHei';
                ctx.textAlign = 'left';
                ctx.fillText(`${d.do.toFixed(2)}`, x + (zoneWidth - 20) / 2 + 10, y + 4);
            }
        }
        
        this.drawLegend(ctx, w, h);
    }
    
    drawAxes(ctx, w, h, zoneWidth, depthHeight, maxDepth, zones) {
        ctx.strokeStyle = '#64748b';
        ctx.lineWidth = 2;
        
        ctx.beginPath();
        ctx.moveTo(100, 60);
        ctx.lineTo(100, 60 + depthHeight);
        ctx.stroke();
        
        ctx.beginPath();
        ctx.moveTo(100, 60 + depthHeight);
        ctx.lineTo(w - 50, 60 + depthHeight);
        ctx.stroke();
        
        ctx.fillStyle = '#94a3b8';
        ctx.font = '12px Microsoft YaHei';
        ctx.textAlign = 'right';
        
        for (let d = 0; d <= maxDepth; d += 1) {
            const y = 60 + (d / maxDepth) * depthHeight;
            ctx.fillText(`${d}m`, 90, y + 4);
            
            ctx.strokeStyle = 'rgba(100, 116, 139, 0.3)';
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.moveTo(100, y);
            ctx.lineTo(w - 50, y);
            ctx.stroke();
        }
        
        ctx.textAlign = 'center';
        for (let zi = 0; zi < zones.length; zi++) {
            const x = 100 + zi * zoneWidth + (zoneWidth - 20) / 2;
            ctx.fillStyle = '#cbd5e1';
            ctx.font = 'bold 13px Microsoft YaHei';
            ctx.fillText(CONFIG.LOCATION_NAMES[zones[zi]], x, 45);
        }
        
        ctx.save();
        ctx.translate(30, h / 2);
        ctx.rotate(-Math.PI / 2);
        ctx.fillStyle = '#94a3b8';
        ctx.font = '14px Microsoft YaHei';
        ctx.textAlign = 'center';
        ctx.fillText('深度 (m)', 0, 0);
        ctx.restore();
    }
    
    drawZoneTank(ctx, x, y, width, height, zone) {
        const gradient = ctx.createLinearGradient(x, y, x, y + height);
        gradient.addColorStop(0, 'rgba(30, 58, 95, 0.3)');
        gradient.addColorStop(1, 'rgba(15, 23, 42, 0.5)');
        
        ctx.fillStyle = gradient;
        ctx.strokeStyle = '#475569';
        ctx.lineWidth = 2;
        
        ctx.beginPath();
        ctx.rect(x, y, width, height);
        ctx.fill();
        ctx.stroke();
        
        if (zone.startsWith('aerobic')) {
            const time = Date.now() / 1000;
            for (let i = 0; i < 8; i++) {
                const bx = x + 20 + (i % 4) * (width - 40) / 3;
                const by = y + height - ((time * 50 + i * 30) % height);
                const br = 3 + Math.sin(time * 2 + i) * 2;
                
                ctx.fillStyle = 'rgba(255, 255, 255, 0.4)';
                ctx.beginPath();
                ctx.arc(bx, by, br, 0, Math.PI * 2);
                ctx.fill();
            }
        }
        
        if (zone === 'anaerobic' || zone === 'anoxic') {
            ctx.fillStyle = 'rgba(239, 68, 68, 0.8)';
            ctx.font = 'bold 11px Microsoft YaHei';
            ctx.textAlign = 'center';
            ctx.fillText('● 搅拌', x + width / 2, y + 20);
        }
    }
    
    drawLegend(ctx, w, h) {
        const legendX = w - 200;
        const legendY = h - 50;
        
        ctx.fillStyle = 'rgba(30, 41, 59, 0.9)';
        ctx.strokeStyle = '#475569';
        ctx.lineWidth = 1;
        this.roundRect(ctx, legendX, legendY, 180, 40, 6);
        ctx.fill();
        ctx.stroke();
        
        const doRanges = [
            { color: '#2c3e50', label: '<0.5' },
            { color: '#e74c3c', label: '0.5-1.5' },
            { color: '#f39c12', label: '1.5-2.0' },
            { color: '#27ae60', label: '2.0-3.0' },
            { color: '#3498db', label: '>3.0' }
        ];
        
        let x = legendX + 10;
        for (const range of doRanges) {
            ctx.fillStyle = range.color;
            ctx.fillRect(x, legendY + 12, 25, 16);
            
            ctx.fillStyle = '#94a3b8';
            ctx.font = '10px Microsoft YaHei';
            ctx.textAlign = 'center';
            ctx.fillText(range.label, x + 12, legendY + 35);
            
            x += 34;
        }
        
        ctx.fillStyle = '#cbd5e1';
        ctx.font = 'bold 11px Microsoft YaHei';
        ctx.textAlign = 'left';
        ctx.fillText('DO (mg/L):', legendX + 10, legendY + 8);
    }
    
    roundRect(ctx, x, y, width, height, radius) {
        ctx.beginPath();
        ctx.moveTo(x + radius, y);
        ctx.lineTo(x + width - radius, y);
        ctx.quadraticCurveTo(x + width, y, x + width, y + radius);
        ctx.lineTo(x + width, y + height - radius);
        ctx.quadraticCurveTo(x + width, y + height, x + width - radius, y + height);
        ctx.lineTo(x + radius, y + height);
        ctx.quadraticCurveTo(x, y + height, x, y + height - radius);
        ctx.lineTo(x, y + radius);
        ctx.quadraticCurveTo(x, y, x + radius, y);
        ctx.closePath();
    }
    
    animate() {
        this.draw();
        requestAnimationFrame(() => this.animate());
    }
    
    updateProfileData(data) {
        this.profileData = data;
        this.draw();
    }
}
