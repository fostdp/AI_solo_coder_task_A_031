const CONFIG = {
    API_BASE_URL: 'http://localhost:8080/api',
    WS_URL: 'ws://localhost:8080/ws',
    
    SENSOR_TYPES: {
        DO: { name: '溶解氧', unit: 'mg/L', color: '#3b82f6', setpoint: 2.0 },
        NH3: { name: '氨氮', unit: 'mg/L', color: '#ef4444', setpoint: 1.5 },
        NO3: { name: '硝氮', unit: 'mg/L', color: '#f59e0b', setpoint: 10.0 },
        PO4: { name: '磷酸盐', unit: 'mg/L', color: '#8b5cf6', setpoint: 0.5 },
        COD: { name: 'COD', unit: 'mg/L', color: '#10b981', setpoint: 300.0 },
        FLOW: { name: '流量', unit: 'm³/d', color: '#ec4899', setpoint: 300000.0 }
    },
    
    DEVIATION_LEVELS: {
        GREEN: { max: 10, color: '#10b981', name: '正常' },
        YELLOW: { max: 20, color: '#f59e0b', name: '警告' },
        RED: { max: Infinity, color: '#ef4444', name: '异常' }
    },
    
    LOCATION_NAMES: {
        anaerobic: '厌氧池',
        anoxic: '缺氧池',
        aerobic1: '好氧池1段',
        aerobic2: '好氧池2段',
        aerobic3: '好氧池3段',
        effluent: '出水',
        influent: '进水'
    },
    
    PROCESS_SECTIONS: [
        { id: 'coarse_bar', name: '粗格栅', type: 'pre_treatment', x: 50, y: 250, width: 80, height: 100 },
        { id: 'fine_bar', name: '细格栅', type: 'pre_treatment', x: 150, y: 250, width: 80, height: 100 },
        { id: 'grit_chamber', name: '沉砂池', type: 'pre_treatment', x: 250, y: 250, width: 80, height: 100 },
        { id: 'primary_settler', name: '初沉池', type: 'primary_treatment', x: 350, y: 250, width: 100, height: 100 },
        { id: 'anaerobic', name: '厌氧池', type: 'biological', x: 100, y: 400, width: 120, height: 150 },
        { id: 'anoxic', name: '缺氧池', type: 'biological', x: 240, y: 400, width: 120, height: 150 },
        { id: 'aerobic1', name: '好氧池1段', type: 'biological', x: 380, y: 400, width: 140, height: 150 },
        { id: 'aerobic2', name: '好氧池2段', type: 'biological', x: 540, y: 400, width: 140, height: 150 },
        { id: 'aerobic3', name: '好氧池3段', type: 'biological', x: 700, y: 400, width: 140, height: 150 },
        { id: 'secondary_settler', name: '二沉池', type: 'secondary_treatment', x: 860, y: 400, width: 120, height: 150 },
        { id: 'advanced_treatment', name: '深度处理', type: 'advanced_treatment', x: 1000, y: 400, width: 100, height: 150 }
    ],
    
    BIOLOGICAL_ZONES: ['anaerobic', 'anoxic', 'aerobic1', 'aerobic2', 'aerobic3'],
    
    COLORS: {
        primary: '#3b82f6',
        secondary: '#8b5cf6',
        success: '#10b981',
        warning: '#f59e0b',
        danger: '#ef4444',
        info: '#06b6d4',
        dark: '#1e293b',
        darker: '#0f172a',
        light: '#e4e4e7',
        muted: '#64748b'
    },
    
    KPI_TARGETS: {
        energyPerTon: 0.35,
        carbonPerTon: 0.25,
        tnRemovalRate: 80,
        nh3RemovalRate: 90,
        tpRemovalRate: 90,
        waterQuality: 85
    }
};
