const CONFIG = {
    API_BASE: '/api/v1',
    WS_URL: `ws://${window.location.host}/api/v1/ws`,
    REFRESH_INTERVAL: 10000,
    COLORS: {
        normal: '#2ecc71',
        warning: '#f1c40f',
        danger: '#e74c3c',
        offline: '#7f8c8d',
        primary: '#00d4ff',
        secondary: '#00ff88'
    },
    SENSOR_TYPES: {
        'DO': { name: '溶解氧', unit: 'mg/L', setpoint: 2.0 },
        'NH3': { name: '氨氮', unit: 'mg/L', setpoint: 1.5 },
        'NO3': { name: '硝氮', unit: 'mg/L', setpoint: 5.0 },
        'PO4': { name: '磷酸盐', unit: 'mg/L', setpoint: 2.0 },
        'COD': { name: 'COD', unit: 'mg/L', setpoint: 300 },
        'TN': { name: '总氮', unit: 'mg/L', setpoint: 10 },
        'TP': { name: '总磷', unit: 'mg/L', setpoint: 0.5 },
        'FLOW': { name: '流量', unit: 'm³/h', setpoint: 1250 }
    },
    STAGE_NAMES: {
        'coarse_grate': '粗格栅',
        'fine_grate': '细格栅',
        'grit_chamber': '沉砂池',
        'primary_settling': '初沉池',
        'anaerobic': '厌氧池',
        'anoxic': '缺氧池',
        'aerobic': '好氧池',
        'secondary_settling': '二沉池',
        'advanced_treatment': '深度处理',
        'effluent': '出水'
    },
    THRESHOLDS: {
        WARNING: 10,
        DANGER: 20
    }
};

function getSensorColor(sensor) {
    if (sensor.status === 'offline' || !sensor.timestamp || 
        (new Date() - new Date(sensor.timestamp)) > 10 * 60 * 1000) {
        return CONFIG.COLORS.offline;
    }
    
    const deviation = Math.abs((sensor.value - sensor.setpoint) / sensor.setpoint * 100);
    if (deviation > CONFIG.THRESHOLDS.DANGER) {
        return CONFIG.COLORS.danger;
    } else if (deviation > CONFIG.THRESHOLDS.WARNING) {
        return CONFIG.COLORS.warning;
    }
    return CONFIG.COLORS.normal;
}

function formatValue(value, type) {
    if (value === null || value === undefined || isNaN(value)) return '--';
    const sensorType = CONFIG.SENSOR_TYPES[type];
    const decimals = sensorType && sensorType.setpoint < 1 ? 3 : 
                     sensorType && sensorType.setpoint < 10 ? 2 : 1;
    return value.toFixed(decimals);
}

function formatTime(date) {
    if (!date) return '--';
    const d = new Date(date);
    return d.toLocaleString('zh-CN', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
    });
}
