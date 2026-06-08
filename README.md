# 城市污水处理厂精确曝气与脱氮除磷优化系统

## 系统概述

本系统是一套完整的城市污水处理厂精确曝气与脱氮除磷优化全栈应用，针对日处理能力30万吨的污水处理厂设计，实现了从数据采集、过程控制到智能优化的全流程管理。

### 核心功能

1. **传感器数据采集**：80个传感器（30台DO、20台NH3、15台NO3、10台PO4、5台其他）每2分钟通过4G DTU上报数据
2. **精确曝气控制**：基于PID+前馈控制算法，实时计算各廊道曝气量设定值
3. **碳源投加优化**：基于反硝化动力学模型，计算最优碳源投加量
4. **两级告警系统**：一级出水超标告警、二级设备故障告警，支持短信+WebSocket双通道推送
5. **可视化监控**：Canvas绘制生化池剖面图和工艺段平面图，实时显示传感器状态和参数趋势

### 工艺段

- 粗格栅 → 细格栅 → 沉砂池 → 初沉池 → 生化池（厌氧-缺氧-好氧三段）→ 二沉池 → 深度处理 → 出水

---

## 技术栈

### 后端
- **语言**：Go 1.21
- **Web框架**：Gin v1.9.1
- **时序数据库**：InfluxDB 1.x
- **消息协议**：MQTT (Eclipse Paho)
- **实时通信**：WebSocket (Gorilla)
- **配置管理**：Viper v1.18.2
- **日志**：Zap v1.27.0
- **定时任务**：Cron v3.0.1

### 前端
- **原生JavaScript** (ES6+)
- **HTML5 Canvas** 2D API
- **WebSocket** 实时通信
- **CSS3** 响应式设计

---

## 项目结构

```
AI_solo_coder_task_A_031/
├── backend/                          # Go后端代码
│   ├── cmd/
│   │   ├── server/                   # 主服务入口
│   │   │   └── main.go
│   │   ├── dtu-simulator/            # 4G DTU传感器模拟器
│   │   │   └── main.go
│   │   └── plc-simulator/            # PLC模拟器
│   │       └── main.go
│   ├── internal/
│   │   ├── config/                   # 配置管理
│   │   │   └── config.go
│   │   ├── models/                   # 数据模型
│   │   │   ├── models.go
│   │   │   └── sensors.go            # 传感器初始化配置
│   │   ├── influxdb/                 # InfluxDB客户端
│   │   │   └── influxdb.go
│   │   ├── mqtt/                     # MQTT客户端
│   │   │   └── mqtt.go
│   │   ├── websocket/                # WebSocket服务
│   │   │   └── websocket.go
│   │   ├── controller/               # 控制算法
│   │   │   ├── aeration.go           # 精确曝气控制器
│   │   │   └── carbon.go             # 碳源投加优化器
│   │   ├── alarm/                    # 告警管理
│   │   │   └── alarm.go
│   │   └── api/                      # RESTful API
│   │       └── api.go
│   ├── pkg/
│   │   └── pid/                      # PID控制器库
│   │       └── pid.go
│   ├── config.yaml                   # 系统配置文件
│   └── go.mod                        # Go依赖管理
├── frontend/                         # 前端代码
│   ├── index.html                    # 主页面
│   ├── css/
│   │   └── style.css                 # 样式文件
│   └── js/
│       ├── config.js                 # 前端配置
│       ├── api.js                    # API调用封装
│       ├── canvas.js                 # Canvas绘制模块
│       ├── trend.js                  # 趋势图绘制模块
│       ├── control.js                # 控制面板逻辑
│       ├── alarm.js                  # 告警管理逻辑
│       ├── websocket.js              # WebSocket客户端
│       └── app.js                    # 应用入口
└── scripts/
    └── influxdb/
        └── init.iql                  # InfluxDB初始化脚本
```

---

## 环境要求

### 基础设施

| 软件 | 版本要求 | 说明 |
|------|---------|------|
| Go | >= 1.21 | 后端编译运行 |
| InfluxDB | >= 1.8 | 时序数据存储 |
| Mosquitto | >= 2.0 | MQTT Broker |
| 浏览器 | Chrome >= 90 / Firefox >= 88 | 前端运行 |

### 端口分配

| 服务 | 端口 | 说明 |
|------|------|------|
| API Server | 8080 | HTTP API和WebSocket |
| InfluxDB | 8086 | 时序数据库 |
| Mosquitto | 1883 | MQTT Broker |

---

## 快速开始

### 1. 安装基础设施

#### InfluxDB (Windows)
```powershell
# 下载安装包或使用Chocolatey
choco install influxdb --version=1.8.10

# 启动服务
influxd -config influxdb.conf
```

#### Mosquitto (Windows)
```powershell
# 使用Chocolatey安装
choco install mosquitto

# 启动服务
net start mosquitto
```

#### Go 1.21
```powershell
# 下载并安装Go 1.21
# https://golang.org/dl/

# 验证安装
go version
```

### 2. 初始化InfluxDB

```powershell
# 进入InfluxDB CLI
influx

# 执行初始化脚本
influx -import -path=scripts/influxdb/init.iql -precision=s
```

或手动执行：
```sql
CREATE DATABASE sewage_plant WITH DURATION 365d REPLICATION 1 SHARD DURATION 7d NAME retention_365d;
USE sewage_plant;
-- 执行init.iql中的连续查询创建语句
```

### 3. 配置系统参数

编辑 `backend/config.yaml`：

```yaml
server:
  port: 8080
  mode: release

influxdb:
  addr: http://localhost:8086
  database: sewage_plant
  username: admin
  password: admin
  retention_policy: retention_365d
  precision: s

mqtt:
  broker: tcp://localhost:1883
  client_id: sewage_server
  username: admin
  password: admin

controller:
  aeration:
    do_setpoint: 2.0           # DO设定值 (mg/L)
    nh3_setpoint: 1.5          # NH3设定值 (mg/L)
    kp: 0.6                    # 比例系数
    ki: 0.15                   # 积分系数
    kd: 0.05                   # 微分系数
    feedforward_gain: 0.3      # 前馈增益
    min_air_flow: 5.0          # 最小曝气量 (m³/min)
    max_air_flow: 50.0         # 最大曝气量 (m³/min)
    control_interval: 30       # 控制间隔 (秒)
  carbon:
    tn_removal_target: 85.0    # 总氮去除目标 (%)
    cod_tn_ratio: 4.5          # 反硝化所需COD/TN比
    min_dosing_rate: 0.0       # 最小投加量 (mg/L)
    max_dosing_rate: 50.0      # 最大投加量 (mg/L)

alarm:
  level1:
    nh3_threshold: 5.0         # 出水NH3告警阈值 (mg/L)
    tn_threshold: 15.0         # 出水TN告警阈值 (mg/L)
    duration_minutes: 30       # 持续超标时间 (分钟)
  level2:
    offline_threshold_minutes: 5  # 传感器离线阈值
  sms_gateway:
    enabled: false
    api_url: ""
    api_key: ""
```

### 4. 编译运行后端

#### 编译主服务
```powershell
cd backend
go mod download
go build -o ../bin/server.exe ./cmd/server
```

#### 编译DTU模拟器
```powershell
go build -o ../bin/dtu-simulator.exe ./cmd/dtu-simulator
```

#### 编译PLC模拟器
```powershell
go build -o ../bin/plc-simulator.exe ./cmd/plc-simulator
```

### 5. 启动系统

#### 启动主服务
```powershell
cd bin
./server.exe
```

#### 启动DTU传感器模拟器 (新终端)
```powershell
# 每2分钟上报80个传感器数据
$env:API_URL="http://localhost:8080/api/v1"
$env:INTERVAL_SECONDS=120
./dtu-simulator.exe
```

#### 启动PLC模拟器 (新终端)
```powershell
$env:MQTT_BROKER="tcp://localhost:1883"
./plc-simulator.exe
```

### 6. 访问前端

直接在浏览器中打开 `frontend/index.html`

或使用简单HTTP服务器：
```powershell
cd frontend
python -m http.server 8081
# 浏览器访问 http://localhost:8081
```

---

## 核心算法说明

### 1. 精确曝气控制模型

**控制策略**：PID + 前馈复合控制

```
曝气量设定值 = PID(DO实际值 - DO设定值) + 前馈(NH3实际值 - NH3设定值)
```

**PID控制器**（抗积分饱和）：
- 比例项：快速响应DO偏差
- 积分项：消除稳态误差，带抗饱和保护
- 微分项：预测DO变化趋势，抑制超调

**前馈补偿**：
- 基于NH3浓度偏差计算前馈量
- 提前调整曝气量，提高响应速度
- 前馈增益：0.3

**控制目标**：
- 好氧池末端DO：1.5-2.5 mg/L (设定值2.0 mg/L)
- 好氧池末端NH3：1-2 mg/L (设定值1.5 mg/L)

### 2. 碳源投加优化模型

**算法原理**：基于反硝化化学计量关系

```
所需碳源量 = (NO3浓度 × 反硝化需碳量 - 进水可利用COD) × 安全系数

其中：
- 反硝化需碳量：4.5 kg COD/kg NO3-N
- 安全系数：1.1-1.2（根据进水水质波动调整）
```

**优化目标**：
- 总氮去除率最大化
- 碳源不浪费（避免过度投加）
- 出水TN稳定达标

### 3. 告警判定逻辑

**一级告警（出水超标）**：
- 出水NH3 > 5.0 mg/L 或 TN > 15.0 mg/L
- 持续时间 ≥ 30分钟
- 推送通道：短信 + WebSocket

**二级告警（设备故障）**：
- 曝气风机故障（PLC状态反馈）
- DO传感器离线（>5分钟无数据）
- 推送通道：WebSocket + 系统通知

---

## API接口说明

### 传感器数据接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/sensors` | 获取所有传感器最新数据 |
| GET | `/api/v1/sensors/info` | 获取传感器配置信息 |
| GET | `/api/v1/sensors/:id` | 获取单个传感器数据 |
| GET | `/api/v1/sensors/:id/trend?hours=6` | 获取传感器趋势数据 |
| POST | `/api/v1/sensors/data` | DTU上报传感器数据 |

### 控制接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/control/aeration` | 获取曝气控制状态 |
| GET | `/api/v1/control/aeration/:section` | 获取单廊道曝气状态 |
| POST | `/api/v1/control/aeration/setpoint` | 设置DO/NH3设定值 |
| POST | `/api/v1/control/aeration/tuning` | 设置PID参数 |
| GET | `/api/v1/control/carbon` | 获取碳源投加状态 |
| POST | `/api/v1/control/carbon/target` | 设置TN去除目标 |

### 告警接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/alerts` | 获取活动告警列表 |
| GET | `/api/v1/alerts/level/:level` | 按级别获取告警 |
| POST | `/api/v1/alerts/:id/ack` | 确认告警 |

### 指标接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/metrics` | 获取最新关键指标 |
| GET | `/api/v1/metrics/trend/:metric?days=7` | 获取指标趋势数据 |

### WebSocket接口

| 路径 | 说明 |
|------|------|
| `/api/v1/ws` | WebSocket连接，实时推送传感器数据、告警、控制状态、指标 |

**消息格式**：
```json
{
    "type": "sensor_data",
    "data": {
        "id": "DO-AER-01",
        "type": "DO",
        "value": 2.15,
        "timestamp": "2024-01-15T10:30:00Z"
    }
}
```

---

## 前端功能说明

### 1. 生化池剖面图

- 展示厌氧池、缺氧池、好氧池（6条廊道）的剖面结构
- 按实际位置标注80个传感器圆点
- 颜色根据参数偏离设定值变化：
  - 🟢 绿色：偏差 < 10%（正常）
  - 🟡 黄色：偏差 10%-20%（预警）
  - 🔴 红色：偏差 > 20%（报警）
  - ⚪ 灰色：传感器离线

### 2. 工艺段平面图

- 完整展示从粗格栅到出水的10个工艺段
- 关键工艺段标注核心传感器
- 支持点击查看各工艺段详细参数

### 3. 传感器趋势分析

点击任意传感器圆点，弹出6小时趋势分析面板：
- 实时参数值、设定值、偏差百分比
- 6小时历史趋势曲线
- 设定值参考线
- 最大值、最小值、平均值统计

### 4. 曝气控制面板

- PID参数在线调整（Kp, Ki, Kd）
- DO和NH3设定值调整
- 30条廊道实时曝气状态展示
- 曝气量和阀门开度实时数据

### 5. 碳源投加控制

- TN去除率目标设定
- 当前投加量和实际投加量展示
- 预计TN去除效果预估

### 6. 关键指标展示

侧边栏实时展示：
- 吨水电耗 (kWh/吨)
- 碳源单耗 (kg/千吨)
- 总氮去除率 (%)
- 总磷去除率 (%)

支持切换查看各指标的历史趋势。

### 7. 告警管理

- 实时展示活动告警列表
- 一级、二级告警分级显示
- 告警确认功能
- 声音提醒和桌面通知

---

## MQTT主题规范

### 上报主题

| 主题 | QoS | 说明 |
|------|-----|------|
| `sensor/data` | 1 | 传感器数据上报 |
| `plc/status` | 1 | PLC设备状态上报 |
| `dtu/heartbeat` | 0 | DTU心跳报文 |

### 下发主题

| 主题 | QoS | 说明 |
|------|-----|------|
| `sewage/control/aeration/+/set` | 1 | 曝气控制指令（+为廊道号） |
| `sewage/control/carbon/set` | 1 | 碳源投加控制指令 |
| `sewage/control/valve/+/set` | 1 | 阀门控制指令 |

### 消息格式示例

**传感器上报**：
```json
{
    "id": "DO-AER-01",
    "type": "DO",
    "stage": "aerobic",
    "section": 1,
    "value": 2.15,
    "setpoint": 2.0,
    "timestamp": "2024-01-15T10:30:00Z",
    "dtu_id": "DTU-0001",
    "status": "online"
}
```

**曝气控制指令**：
```json
{
    "section": 1,
    "air_flow_set": 25.5,
    "valve_open": 68,
    "do_setpoint": 2.0,
    "timestamp": "2024-01-15T10:30:00Z"
}
```

---

## InfluxDB数据存储

### Measurement说明

| Measurement | 说明 | 采样频率 |
|------------|------|---------|
| `sensor_data` | 传感器原始数据 | 2分钟 |
| `sensor_data_5m` | 传感器5分钟聚合 | 5分钟 |
| `sensor_data_1h` | 传感器1小时聚合 | 1小时 |
| `aeration_control` | 曝气控制数据 | 30秒 |
| `aeration_control_1h` | 曝气控制1小时聚合 | 1小时 |
| `carbon_dosing` | 碳源投加数据 | 1分钟 |
| `carbon_dosing_1h` | 碳源投加1小时聚合 | 1小时 |
| `key_metrics` | 关键指标 | 1分钟 |
| `key_metrics_1h` | 关键指标1小时聚合 | 1小时 |
| `key_metrics_1d` | 关键指标1天聚合 | 1天 |

### 数据保留策略

- 原始数据：365天
- 5分钟聚合：365天
- 1小时聚合：365天
- 1天聚合：永久

### Tag说明

**sensor_data**：
- `sensor_id`：传感器ID
- `type`：传感器类型 (DO/NH3/NO3/PO4等)
- `stage`：工艺段
- `section`：廊道/段号

---

## 故障排查

### 常见问题

**1. 后端启动失败，InfluxDB连接超时**
```
检查InfluxDB服务是否启动：netstat -ano | findstr 8086
检查config.yaml中的addr配置是否正确
```

**2. MQTT连接失败**
```
检查Mosquitto服务是否启动：netstat -ano | findstr 1883
检查用户名密码配置
```

**3. 前端无法获取数据**
```
打开浏览器开发者工具，检查Network面板
确认API_BASE配置是否正确
检查后端服务是否正常运行
```

**4. WebSocket连接断开**
```
检查WebSocket服务是否正常
确认防火墙未阻止8080端口
系统会自动尝试重连（最多10次）
```

**5. 传感器数据不更新**
```
检查DTU模拟器是否正常运行
检查API接口是否正常返回数据
查看后端日志确认数据是否写入InfluxDB
```

### 日志查看

后端服务日志输出到控制台，主要日志级别：
- INFO：正常运行信息
- WARN：警告信息
- ERROR：错误信息

DTU模拟器使用 `-v` 参数可输出详细调试日志：
```powershell
./dtu-simulator.exe -v
```

---

## 性能指标

在标准硬件配置（4核CPU，8GB内存）下：

| 指标 | 目标值 |
|------|--------|
| API响应时间 | < 100ms |
| WebSocket推送延迟 | < 500ms |
| 控制循环执行时间 | < 100ms |
| 数据写入InfluxDB | < 50ms |
| 前端Canvas渲染帧率 | > 30fps |
| 支持并发WebSocket连接 | > 100 |

---

## 扩展建议

1. **增加历史数据对比功能**：同比、环比数据分析
2. **接入实际PLC设备**：替换PLC模拟器，对接真实控制系统
3. **增加AI预测模型**：基于历史数据预测进水水质变化
4. **移动端适配**：开发移动端监控APP
5. **报表生成**：自动生成日报、月报、年报
6. **多租户支持**：支持多厂区统一管理

---

## 许可证

Copyright © 2024 城市污水处理厂精确曝气系统

---

## 技术支持

如有问题，请查看代码中的注释或参考各模块文档。
