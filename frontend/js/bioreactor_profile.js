class BiologicalProfile {
    constructor(canvasId) {
        this.canvas = document.getElementById(canvasId);
        if (!this.canvas) {
            console.error(`Canvas element not found: ${canvasId}`);
            return;
        }
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
                ctx.strokeStyle = d.color || '#333';
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
