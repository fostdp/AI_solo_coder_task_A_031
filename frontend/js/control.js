const ControlPanel = {
    aerationStatus: [],
    carbonStatus: null,
    currentMetric: 'power_consumption',

    init() {
        this.bindEvents();
        this.loadInitialData();
        this.startAutoRefresh();
    },

    bindEvents() {
        document.getElementById('do-slider').addEventListener('input', (e) => {
            document.getElementById('do-setpoint').textContent = e.target.value;
        });

        document.getElementById('nh3-slider').addEventListener('input', (e) => {
            document.getElementById('nh3-setpoint').textContent = e.target.value;
        });

        document.getElementById('kp-slider').addEventListener('input', (e) => {
            document.getElementById('kp-value').textContent = e.target.value;
        });

        document.getElementById('ki-slider').addEventListener('input', (e) => {
            document.getElementById('ki-value').textContent = e.target.value;
        });

        document.getElementById('kd-slider').addEventListener('input', (e) => {
            document.getElementById('kd-value').textContent = e.target.value;
        });

        document.getElementById('tn-target-slider').addEventListener('input', (e) => {
            document.getElementById('tn-target').textContent = e.target.value;
        });

        document.getElementById('apply-aeration').addEventListener('click', () => {
            this.applyAerationParams();
        });

        document.getElementById('apply-carbon').addEventListener('click', () => {
            this.applyCarbonParams();
        });

        document.querySelectorAll('.trend-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                document.querySelectorAll('.trend-btn').forEach(b => b.classList.remove('active'));
                e.target.classList.add('active');
                this.currentMetric = e.target.dataset.metric;
                this.loadMetricsTrend();
            });
        });
    },

    async loadInitialData() {
        try {
            await Promise.all([
                this.loadAerationStatus(),
                this.loadCarbonStatus(),
                this.loadMetricsTrend()
            ]);
        } catch (error) {
            console.error('加载控制数据失败:', error);
        }
    },

    async loadAerationStatus() {
        try {
            const data = await API.getAerationStatus();
            this.aerationStatus = data.sections || [];
            this.renderAerationStatus();
            
            if (data.do_setpoint) {
                document.getElementById('do-slider').value = data.do_setpoint;
                document.getElementById('do-setpoint').textContent = data.do_setpoint;
            }
            if (data.nh3_setpoint) {
                document.getElementById('nh3-slider').value = data.nh3_setpoint;
                document.getElementById('nh3-setpoint').textContent = data.nh3_setpoint;
            }
            if (data.kp !== undefined) {
                document.getElementById('kp-slider').value = data.kp;
                document.getElementById('kp-value').textContent = data.kp;
            }
            if (data.ki !== undefined) {
                document.getElementById('ki-slider').value = data.ki;
                document.getElementById('ki-value').textContent = data.ki;
            }
            if (data.kd !== undefined) {
                document.getElementById('kd-slider').value = data.kd;
                document.getElementById('kd-value').textContent = data.kd;
            }
        } catch (error) {
            console.error('加载曝气状态失败:', error);
        }
    },

    async loadCarbonStatus() {
        try {
            const data = await API.getCarbonStatus();
            this.carbonStatus = data;
            this.renderCarbonStatus();
            
            if (data.tn_removal_target) {
                document.getElementById('tn-target-slider').value = data.tn_removal_target;
                document.getElementById('tn-target').textContent = data.tn_removal_target;
            }
        } catch (error) {
            console.error('加载碳源状态失败:', error);
        }
    },

    renderAerationStatus() {
        const container = document.getElementById('aeration-status');
        if (!container) return;

        container.innerHTML = '';

        this.aerationStatus.forEach((section, index) => {
            const statusClass = section.status === 'normal' ? 'status-normal' :
                               section.status === 'warning' ? 'status-warning' :
                               section.status === 'error' ? 'status-error' : 'status-offline';

            const card = document.createElement('div');
            card.className = `aeration-card ${statusClass}`;
            card.innerHTML = `
                <div class="aeration-header">
                    <span class="section-name">廊道 ${section.section || (index + 1)}</span>
                    <span class="status-dot ${section.status || 'normal'}"></span>
                </div>
                <div class="aeration-data">
                    <div><span>DO:</span> ${formatValue(section.do, 'DO')} mg/L</div>
                    <div><span>NH3:</span> ${formatValue(section.nh3, 'NH3')} mg/L</div>
                    <div><span>曝气量:</span> ${(section.air_flow || 0).toFixed(1)} m³/min</div>
                    <div><span>阀门开度:</span> ${(section.valve_open || 0).toFixed(0)}%</div>
                </div>
            `;
            container.appendChild(card);
        });
    },

    renderCarbonStatus() {
        if (!this.carbonStatus) return;

        document.getElementById('dosing-rate').textContent = 
            (this.carbonStatus.dosing_rate || 0).toFixed(1);
        document.getElementById('dosing-actual').textContent = 
            (this.carbonStatus.actual_dosing || 0).toFixed(1);
        document.getElementById('tn-removal').textContent = 
            (this.carbonStatus.estimated_tn_removal || 0).toFixed(1);
    },

    async applyAerationParams() {
        try {
            const doSetpoint = parseFloat(document.getElementById('do-slider').value);
            const nh3Setpoint = parseFloat(document.getElementById('nh3-slider').value);
            const kp = parseFloat(document.getElementById('kp-slider').value);
            const ki = parseFloat(document.getElementById('ki-slider').value);
            const kd = parseFloat(document.getElementById('kd-slider').value);

            await Promise.all([
                API.setAerationSetpoint(doSetpoint, nh3Setpoint),
                API.setAerationTuning(kp, ki, kd)
            ]);

            this.showNotification('曝气参数已应用', 'success');
        } catch (error) {
            console.error('应用曝气参数失败:', error);
            this.showNotification('应用曝气参数失败', 'error');
        }
    },

    async applyCarbonParams() {
        try {
            const tnTarget = parseFloat(document.getElementById('tn-target-slider').value);
            await API.setCarbonTarget(tnTarget);
            this.showNotification('碳源参数已应用', 'success');
        } catch (error) {
            console.error('应用碳源参数失败:', error);
            this.showNotification('应用碳源参数失败', 'error');
        }
    },

    async loadMetricsTrend() {
        try {
            const data = await API.getMetricsTrend(this.currentMetric, 1);
            SensorTrend.drawMetricsTrend(data, this.currentMetric);
        } catch (error) {
            console.error('加载指标趋势失败:', error);
        }
    },

    updateAerationSection(sectionData) {
        const index = this.aerationStatus.findIndex(s => s.section === sectionData.section);
        if (index >= 0) {
            this.aerationStatus[index] = { ...this.aerationStatus[index], ...sectionData };
        } else {
            this.aerationStatus.push(sectionData);
        }
        this.renderAerationStatus();
    },

    updateCarbonStatus(data) {
        this.carbonStatus = { ...this.carbonStatus, ...data };
        this.renderCarbonStatus();
    },

    showNotification(message, type = 'info') {
        const notification = document.createElement('div');
        notification.className = `notification notification-${type}`;
        notification.textContent = message;
        document.body.appendChild(notification);
        
        setTimeout(() => {
            notification.classList.add('show');
        }, 10);

        setTimeout(() => {
            notification.classList.remove('show');
            setTimeout(() => notification.remove(), 300);
        }, 3000);
    },

    startAutoRefresh() {
        setInterval(() => {
            this.loadAerationStatus();
            this.loadCarbonStatus();
            this.loadMetricsTrend();
        }, CONFIG.REFRESH_INTERVAL);
    }
};
