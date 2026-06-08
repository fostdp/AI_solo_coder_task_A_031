const API = {
    async get(endpoint) {
        const response = await fetch(`${CONFIG.API_BASE}${endpoint}`);
        if (!response.ok) {
            throw new Error(`API Error: ${response.status}`);
        }
        return await response.json();
    },

    async post(endpoint, data) {
        const response = await fetch(`${CONFIG.API_BASE}${endpoint}`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
        if (!response.ok) {
            throw new Error(`API Error: ${response.status}`);
        }
        return await response.json();
    },

    async getAllSensors() {
        return await this.get('/sensors');
    },

    async getSensorInfo() {
        return await this.get('/sensors/info');
    },

    async getSensorData(sensorId) {
        return await this.get(`/sensors/${sensorId}`);
    },

    async getSensorTrend(sensorId, hours = 6) {
        return await this.get(`/sensors/${sensorId}/trend?hours=${hours}`);
    },

    async getAerationStatus() {
        return await this.get('/control/aeration');
    },

    async getAerationSectionStatus(section) {
        return await this.get(`/control/aeration/${section}`);
    },

    async setAerationSetpoint(doSetpoint, nh3Setpoint) {
        return await this.post('/control/aeration/setpoint', {
            do_setpoint: doSetpoint,
            nh3_setpoint: nh3Setpoint
        });
    },

    async setAerationTuning(kp, ki, kd) {
        return await this.post('/control/aeration/tuning', {
            kp: kp,
            ki: ki,
            kd: kd
        });
    },

    async getCarbonStatus() {
        return await this.get('/control/carbon');
    },

    async setCarbonTarget(tnRemovalTarget) {
        return await this.post('/control/carbon/target', {
            tn_removal_target: tnRemovalTarget
        });
    },

    async getActiveAlarms() {
        return await this.get('/alerts');
    },

    async getAlarmsByLevel(level) {
        return await this.get(`/alerts/level/${level}`);
    },

    async acknowledgeAlarm(alarmId) {
        return await this.post(`/alerts/${alarmId}/ack`, {});
    },

    async getKeyMetrics() {
        return await this.get('/metrics');
    },

    async getMetricsTrend(metric, days = 7) {
        return await this.get(`/metrics/trend/${metric}?days=${days}`);
    },

    async getProcessStages() {
        return await this.get('/process/stages');
    }
};
