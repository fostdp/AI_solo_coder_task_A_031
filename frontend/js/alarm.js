const AlarmManager = {
    activeAlarms: [],
    audioContext: null,

    init() {
        this.loadActiveAlarms();
        this.startAutoRefresh();
        this.initAudio();
    },

    initAudio() {
        try {
            this.audioContext = new (window.AudioContext || window.webkitAudioContext)();
        } catch (e) {
            console.warn('Web Audio API not supported');
        }
    },

    playAlarmSound(level) {
        if (!this.audioContext) return;

        const oscillator = this.audioContext.createOscillator();
        const gainNode = this.audioContext.createGain();

        oscillator.connect(gainNode);
        gainNode.connect(this.audioContext.destination);

        if (level === 1) {
            oscillator.frequency.setValueAtTime(800, this.audioContext.currentTime);
            oscillator.frequency.setValueAtTime(600, this.audioContext.currentTime + 0.2);
            oscillator.frequency.setValueAtTime(800, this.audioContext.currentTime + 0.4);
            gainNode.gain.setValueAtTime(0.3, this.audioContext.currentTime);
            gainNode.gain.exponentialRampToValueAtTime(0.01, this.audioContext.currentTime + 0.6);
            oscillator.start(this.audioContext.currentTime);
            oscillator.stop(this.audioContext.currentTime + 0.6);
        } else {
            oscillator.frequency.setValueAtTime(500, this.audioContext.currentTime);
            gainNode.gain.setValueAtTime(0.2, this.audioContext.currentTime);
            gainNode.gain.exponentialRampToValueAtTime(0.01, this.audioContext.currentTime + 0.3);
            oscillator.start(this.audioContext.currentTime);
            oscillator.stop(this.audioContext.currentTime + 0.3);
        }
    },

    async loadActiveAlarms() {
        try {
            const data = await API.getActiveAlarms();
            this.activeAlarms = data.alarms || [];
            this.renderAlarmList();
            this.updateAlarmCount();
        } catch (error) {
            console.error('加载告警失败:', error);
        }
    },

    renderAlarmList() {
        const container = document.getElementById('alarm-list');
        if (!container) return;

        if (this.activeAlarms.length === 0) {
            container.innerHTML = '<div class="no-alarm">暂无告警</div>';
            return;
        }

        container.innerHTML = '';

        this.activeAlarms
            .sort((a, b) => {
                if (a.level !== b.level) return a.level - b.level;
                return new Date(b.triggered_at) - new Date(a.triggered_at);
            })
            .forEach(alarm => {
                const alarmItem = this.createAlarmItem(alarm);
                container.appendChild(alarmItem);
            });
    },

    createAlarmItem(alarm) {
        const item = document.createElement('div');
        item.className = `alarm-item level-${alarm.level} ${alarm.acknowledged ? 'acknowledged' : ''}`;
        item.dataset.id = alarm.id;

        const levelText = alarm.level === 1 ? '一级告警' : '二级告警';
        const levelClass = alarm.level === 1 ? 'level-badge level-1' : 'level-badge level-2';

        const typeNames = {
            'nh3_exceed': '出水氨氮超标',
            'tn_exceed': '出水总氮超标',
            'sensor_offline': '传感器离线',
            'blower_fault': '曝气风机故障',
            'valve_fault': '阀门故障',
            'high_do': '溶解氧过高',
            'low_do': '溶解氧过低'
        };

        const typeName = typeNames[alarm.type] || alarm.type;

        item.innerHTML = `
            <div class="alarm-header">
                <span class="${levelClass}">${levelText}</span>
                <span class="alarm-type">${typeName}</span>
                ${alarm.acknowledged ? '<span class="ack-badge">已确认</span>' : ''}
            </div>
            <div class="alarm-message">${alarm.message}</div>
            <div class="alarm-footer">
                <span class="alarm-time">${formatTime(alarm.triggered_at)}</span>
                ${!alarm.acknowledged ? 
                    `<button class="ack-btn" data-id="${alarm.id}">确认</button>` : 
                    `<span class="ack-info">${alarm.acknowledged_by || '系统'} · ${formatTime(alarm.acknowledged_at)}</span>`
                }
            </div>
        `;

        if (!alarm.acknowledged) {
            const ackBtn = item.querySelector('.ack-btn');
            ackBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.acknowledgeAlarm(alarm.id);
            });
        }

        return item;
    },

    async acknowledgeAlarm(alarmId) {
        try {
            await API.acknowledgeAlarm(alarmId);
            const alarm = this.activeAlarms.find(a => a.id === alarmId);
            if (alarm) {
                alarm.acknowledged = true;
                alarm.acknowledged_at = new Date().toISOString();
                alarm.acknowledged_by = '操作员';
            }
            this.renderAlarmList();
            this.updateAlarmCount();
            this.showNotification('告警已确认', 'success');
        } catch (error) {
            console.error('确认告警失败:', error);
            this.showNotification('确认告警失败', 'error');
        }
    },

    updateAlarmCount() {
        const countEl = document.getElementById('alarm-count');
        if (!countEl) return;

        const unacknowledged = this.activeAlarms.filter(a => !a.acknowledged).length;
        countEl.textContent = unacknowledged;
        countEl.style.display = unacknowledged > 0 ? 'inline-block' : 'none';

        if (unacknowledged > 0) {
            document.title = `(${unacknowledged}) 城市污水处理厂精确曝气系统`;
        } else {
            document.title = '城市污水处理厂精确曝气与脱氮除磷优化系统';
        }
    },

    addAlarm(alarm) {
        const exists = this.activeAlarms.some(a => a.id === alarm.id);
        if (!exists) {
            this.activeAlarms.unshift(alarm);
            this.playAlarmSound(alarm.level);
            this.showNotification(`${alarm.level === 1 ? '【一级告警】' : '【二级告警】'} ${alarm.message}`, 
                alarm.level === 1 ? 'error' : 'warning');
        } else {
            const index = this.activeAlarms.findIndex(a => a.id === alarm.id);
            this.activeAlarms[index] = alarm;
        }
        this.renderAlarmList();
        this.updateAlarmCount();
    },

    updateAlarm(alarmId, updates) {
        const index = this.activeAlarms.findIndex(a => a.id === alarmId);
        if (index >= 0) {
            this.activeAlarms[index] = { ...this.activeAlarms[index], ...updates };
            this.renderAlarmList();
            this.updateAlarmCount();
        }
    },

    removeAlarm(alarmId) {
        this.activeAlarms = this.activeAlarms.filter(a => a.id !== alarmId);
        this.renderAlarmList();
        this.updateAlarmCount();
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
        }, 5000);
    },

    startAutoRefresh() {
        setInterval(() => {
            this.loadActiveAlarms();
        }, CONFIG.REFRESH_INTERVAL);
    }
};
