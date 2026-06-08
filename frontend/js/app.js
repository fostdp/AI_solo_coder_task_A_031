const App = {
    currentView: 'profile',
    metricsRefreshTimer: null,

    init() {
        console.log('系统初始化中...');
        
        this.bindTabEvents();
        this.bindModalEvents();
        this.startClock();
        
        try {
            BioreactorProfile.init((sensor) => this.showSensorTrend(sensor));
        } catch (e) {
            console.error('剖面图模块初始化失败:', e);
        }

        try {
            SensorTrend.init();
        } catch (e) {
            console.error('趋势图模块初始化失败:', e);
        }

        try {
            ControlPanel.init();
        } catch (e) {
            console.error('控制面板初始化失败:', e);
        }

        try {
            AlarmManager.init();
        } catch (e) {
            console.error('告警管理初始化失败:', e);
        }

        try {
            WebSocketClient.init();
        } catch (e) {
            console.error('WebSocket初始化失败:', e);
        }

        this.loadInitialMetrics();
        this.startMetricsRefresh();

        console.log('系统初始化完成');
    },

    bindTabEvents() {
        const tabButtons = document.querySelectorAll('.tab-btn');
        tabButtons.forEach(btn => {
            btn.addEventListener('click', (e) => {
                const view = e.target.dataset.view;
                this.switchView(view);
            });
        });
    },

    switchView(view) {
        document.querySelectorAll('.tab-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.view === view);
        });

        document.querySelectorAll('.tab-content').forEach(content => {
            content.classList.toggle('active', content.id === `view-${view}`);
        });

        this.currentView = view;

        if (view === 'profile' || view === 'plan') {
            setTimeout(() => {
                if (BioreactorProfile) {
                    BioreactorProfile.render();
                }
            }, 100);
        }
    },

    bindModalEvents() {
        const modal = document.getElementById('trend-modal');
        const closeBtn = document.getElementById('close-modal');

        if (closeBtn) {
            closeBtn.addEventListener('click', () => {
                this.closeModal();
            });
        }

        if (modal) {
            modal.addEventListener('click', (e) => {
                if (e.target === modal) {
                    this.closeModal();
                }
            });
        }

        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') {
                this.closeModal();
            }
        });
    },

    closeModal() {
        const modal = document.getElementById('trend-modal');
        if (modal) {
            modal.classList.remove('active');
        }
    },

    startClock() {
        const updateTime = () => {
            const timeEl = document.getElementById('current-time');
            if (timeEl) {
                const now = new Date();
                timeEl.textContent = now.toLocaleString('zh-CN', {
                    year: 'numeric',
                    month: '2-digit',
                    day: '2-digit',
                    hour: '2-digit',
                    minute: '2-digit',
                    second: '2-digit'
                });
            }
        };

        updateTime();
        setInterval(updateTime, 1000);
    },

    async loadInitialMetrics() {
        try {
            const data = await API.getKeyMetrics();
            this.updateMetrics(data);
        } catch (error) {
            console.error('加载关键指标失败:', error);
        }
    },

    updateMetrics(data) {
        if (!data) return;

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

    startMetricsRefresh() {
        if (this.metricsRefreshTimer) {
            clearInterval(this.metricsRefreshTimer);
        }

        this.metricsRefreshTimer = setInterval(async () => {
            try {
                const data = await API.getKeyMetrics();
                this.updateMetrics(data);
            } catch (error) {
                console.error('刷新指标失败:', error);
            }
        }, CONFIG.REFRESH_INTERVAL);
    },

    async showSensorTrend(sensor) {
        try {
            const data = await API.getSensorTrend(sensor.id, 6);
            SensorTrend.showModal(sensor, data);
        } catch (error) {
            console.error('加载传感器趋势失败:', error);
            SensorTrend.showModal(sensor, []);
        }
    }
};

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
        App.init();
    });
} else {
    App.init();
}
