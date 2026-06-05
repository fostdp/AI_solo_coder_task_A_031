# 城市污水处理厂精确曝气与脱氮除磷优化系统

## 项目概述

本系统是一套完整的城市污水处理厂精确曝气与脱氮除磷优化全栈应用，适用于日处理能力30万吨的污水处理厂。系统实现了工艺段可视化、传感器实时监控、精确曝气控制、碳源投加优化、两级告警等核心功能。

## 系统架构

```
┌─────────────────────────────────────────────────────────────────┐
│                     前端 Web 界面 (HTML5 Canvas)                  │
│  工艺流程图、生化池剖面图、传感器状态、KPI趋势、告警面板        │
└─────────────────────────────┬───────────────────────────────────┘
                              │ WebSocket + REST API
┌─────────────────────────────▼───────────────────────────────────┐
│                       Go 后端服务                                │
│  HTTP API、WebSocket、MQTT、InfluxDB操作、PID控制、告警管理    │
└─────────┬───────────────────────────┬───────────────────────────┘
          │ MQTT                      │ InfluxDB
┌─────────▼─────────┐     ┌───────────▼───────────┐
│   MQTT Broker     │     │   InfluxDB 时序数据库  │
│  Mosquitto        │     │   传感器数据存储       │
└─────────▲─────────┘     └───────────────────────┘
          │
    ┌─────┴─────┐
    │           │
┌───▼───┐   ┌───▼───┐
│ 传感器 │   │ PLC  │
│ 模拟器 │   │ 模拟器 │
└───────┘   └───────┘
```

## 工艺段配置

1. **粗格栅** - 去除大颗粒悬浮物
2. **细格栅** - 去除小颗粒悬浮物
3. **沉砂池** - 去除砂粒
4. **初沉池** - 初级沉淀
5. **生化池** - 核心处理单元：
   - 厌氧池 - 磷释放
   - 缺氧池 - 反硝化脱氮
   - 好氧池（A/B/C三段）- 硝化反应、磷吸收
6. **二沉池** - 泥水分离
7. **深度处理** - 进一步净化
8. **出水** - 达标排放

## 传感器配置

| 传感器类型 | 数量 | 分布位置 | 单位 | 设定范围 |
|-----------|------|---------|------|---------|
| 溶解氧(DO) | 30台 | 好氧池A/B/C段，每段10台 | mg/L | 1.5-2.5 |
| 氨氮(NH3) | 20台 | 好氧池A/B/C段 | mg/L | 1.0-2.0 |
| 硝氮(NO3) | 15台 | 缺氧池 | mg/L | 0.5-3.0 |
| 磷酸盐(PO4) | 10台 | 厌氧池 | mg/L | 0.3-1.0 |
| COD | 1台 | 进水 | mg/L | 200-400 |
| 总氮(TN) | 1台 | 出水 | mg/L | 5-15 |
| 氨氮(NH3) | 1台 | 出水 | mg/L | 0.5-1.5 |

**共计：78台传感器，每2分钟通过4G DTU上报数据**

## 控制设备

| 设备类型 | 数量 | 说明 |
|---------|------|------|
| 曝气鼓风机 | 3台 | 对应好氧池A/B/C段 |
| 曝气阀门 | 3台 | 对应好氧池A/B/C段 |
| 碳源投加泵 | 1台 | 碳源投加控制 |
| 碳源阀门 | 1台 | 碳源投加调节 |

## 核心功能

### 1. 精确曝气控制模型

**控制算法：PID + 前馈控制**

- **控制目标**：
  - 好氧池末端氨氮稳定在 1-2 mg/L
  - 溶解氧控制在 1.5-2.5 mg/L

- **控制逻辑**：
  - DO控制器：PID调节，权重70%
  - NH3控制器：PID调节，权重30%
  - 前馈控制：基于氨氮变化率预测曝气需求
  - 控制指令通过MQTT下发到鼓风机和阀门

### 2. 碳源投加优化模型

- **输入**：缺氧池硝氮浓度、进水COD
- **目标**：总氮去除率最大化，碳源不浪费
- **算法**：
  - 物料衡算：计算理论COD需求量
  - 反馈控制：基于出水TN调整投加量
  - 学习因子：根据历史数据优化投加效率

### 3. 两级告警系统

**一级告警（出水超标）**：
- 触发条件：出水氨氮>5mg/L 或 总氮>15mg/L，持续30分钟
- 通知方式：短信 + WebSocket

**二级告警（设备故障）**：
- 触发条件：曝气风机故障 或 DO传感器离线（5分钟无数据）
- 通知方式：短信 + WebSocket

### 4. 前端可视化

- **工艺流程图**：Canvas绘制各工艺段，标注传感器位置
- **生化池剖面图**：展示厌氧-缺氧-好氧结构，曝气状态动画
- **传感器状态**：圆点颜色表示偏离程度
  - 绿色：偏差<10%
  - 黄色：偏差10%-20%
  - 红色：偏差>20%
  - 灰色：离线
- **趋势曲线**：点击传感器显示近6小时参数趋势
- **KPI展示**：吨水电耗、碳源单耗、综合去除率趋势（7天）

## 项目结构

```
AI_solo_coder_task_A_031/
├── backend/                     # Go 后端代码
│   ├── main.go                 # 主程序入口
│   ├── config/
│   │   └── config.go           # 配置管理
│   ├── models/
│   │   └── models.go           # 数据模型
│   ├── influxdb/
│   │   └── client.go           # InfluxDB客户端
│   ├── mqtt/
│   │   └── client.go           # MQTT客户端
│   ├── control/
│   │   ├── aeration.go         # 曝气控制(PID+前馈)
│   │   └── carbon.go           # 碳源投加优化
│   ├── alarm/
│   │   └── alarm.go            # 两级告警系统
│   ├── websocket/
│   │   └── server.go           # WebSocket服务
│   └── api/
│       └── handlers.go         # HTTP API处理器
├── frontend/                    # 前端代码
│   ├── index.html              # 主页面
│   ├── css/
│   │   └── style.css           # 样式文件
│   └── js/
│       └── app.js              # Canvas绘制和交互逻辑
├── scripts/                     # 脚本文件
│   ├── influxdb_init.iql       # InfluxDB初始化脚本
│   ├── sensor_simulator.py     # 4G DTU传感器模拟器
│   ├── plc_simulator.py        # PLC模拟器
│   └── requirements.txt        # Python依赖
├── config.yaml                  # 系统配置文件
├── go.mod                       # Go模块依赖
└── README.md                    # 项目说明
```

## 快速开始

### 1. 环境准备

**必需软件**：
- Go 1.21+
- InfluxDB 1.8+
- Mosquitto (MQTT Broker)
- Python 3.7+ (用于运行模拟器)

### 2. 数据库初始化

```bash
influx -import -path=scripts/influxdb_init.iql
```

### 3. 安装Go依赖

```bash
go mod download
```

### 4. 启动MQTT Broker

```bash
# Linux
mosquitto

# Windows
# 启动Mosquitto服务
```

### 5. 启动后端服务

```bash
go run backend/main.go
```

服务启动后访问：http://localhost:8080

### 6. 启动传感器模拟器（新终端）

```bash
cd scripts
pip install -r requirements.txt
python sensor_simulator.py
```

### 7. 启动PLC模拟器（新终端）

```bash
cd scripts
python plc_simulator.py
```

## API接口文档

### 系统概览
```
GET /api/overview
返回系统概览数据（KPI、告警数、出水水质）
```

### 传感器接口
```
GET /api/sensors              # 获取所有传感器配置
GET /api/sensors/status       # 获取所有传感器状态
GET /api/sensors/:id          # 获取单个传感器最新数据
GET /api/sensors/:id/trend    # 获取传感器趋势数据（默认6小时）
```

### 控制接口
```
GET /api/control/aeration     # 获取曝气控制状态
GET /api/control/carbon       # 获取碳源投加状态
```

### KPI接口
```
GET /api/kpi                  # 获取当前KPI值
GET /api/kpi/:type/trend      # 获取KPI趋势（默认7天）
```

### 告警接口
```
GET /api/alarms               # 获取活跃告警
POST /api/alarms/:id/ack      # 确认告警
```

### 工艺状态接口
```
GET /api/process              # 获取各工艺段状态
```

## MQTT主题规范

### 传感器数据上报
```
Topic: sewage/sensor/{sensor_id}
Payload: {
  "id": "DO-A-1",
  "type": "DO",
  "stage": "aerobic",
  "section": 1,
  "value": 2.0,
  "unit": "mg/L",
  "timestamp": "2024-01-01T12:00:00Z",
  "status": "online",
  "alarm_level": 0
}
```

### 控制指令下发
```
Topic: sewage/command/{device_id}
Payload: {
  "id": "cmd_123456",
  "type": "aeration",
  "target": "blower_1",
  "value": 65.0,
  "unit": "%",
  "timestamp": "2024-01-01T12:00:00Z",
  "source": "aeration_controller"
}
```

### 控制响应上报
```
Topic: sewage/command/response
Payload: {
  "id": "resp_123456",
  "device_id": "blower_1",
  "device_name": "1号曝气风机",
  "status": "completed",
  "message": "1号曝气风机 执行完成",
  "device_state": {...},
  "timestamp": "2024-01-01T12:00:02Z"
}
```

## WebSocket消息规范

连接地址：`ws://localhost:8080/ws`

### 消息类型

1. **sensor_update** - 传感器更新
2. **alarm** - 告警通知
3. **kpi_update** - KPI更新
4. **control_update** - 控制状态更新
5. **heartbeat** - 心跳包

## 模拟器交互命令

### 传感器模拟器
```
anomaly <sensor_id>    # 模拟传感器异常
quit                   # 退出程序
```

### PLC模拟器
```
status              # 显示设备状态
fault <device_id>   # 模拟设备故障
repair <device_id>  # 修复设备故障
history             # 显示命令历史
quit                # 退出程序
```

## 关键技术栈

**后端**：
- Go 1.21
- Gin Web框架
- Paho MQTT客户端
- InfluxDB客户端
- Gorilla WebSocket
- Viper配置管理

**前端**：
- HTML5 Canvas
- 原生JavaScript (ES6+)
- WebSocket实时通信

**数据存储**：
- InfluxDB 时序数据库
- 保留策略：原始数据30天，聚合数据365天
- 连续查询：自动分钟/小时/天级聚合

## 性能指标

- 传感器数据上报：每2分钟78个数据点，每日约5.6万条
- 控制计算周期：曝气30秒，碳源60秒
- WebSocket推送：传感器数据实时推送，KPI每5秒推送
- 支持并发用户：100+

## 告警配置示例

在`config.yaml`中配置：
```yaml
alarm:
  sms:
    api_key: "your_api_key"
    api_secret: "your_api_secret"
    phones:
      - "13800138000"
      - "13900139000"
```

## 常见问题

**Q: MQTT连接失败？**
A: 检查Mosquitto是否启动，端口1883是否被占用。

**Q: InfluxDB连接失败？**
A: 检查InfluxDB是否启动，默认端口8086。执行初始化脚本创建数据库。

**Q: 前端看不到数据？**
A: 确保传感器模拟器正在运行，检查浏览器控制台是否有错误。

**Q: 如何调整控制参数？**
A: 修改`config.yaml`中的control配置，重启后端服务。

## 许可证

MIT License
