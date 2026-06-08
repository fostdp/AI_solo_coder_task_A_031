# 城市污水处理厂精确曝气与脱氮除磷优化系统

## 项目概述

本项目是一个完整的城市污水处理厂精确曝气与脱氮除磷优化系统，适用于日处理能力30万吨的污水处理厂。系统采用Go语言后端、InfluxDB时序数据库、MQTT消息队列、Canvas前端可视化的全栈架构，实现了精确曝气控制、碳源投加优化、实时告警推送等核心功能。

## 系统架构

### 架构图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              前端层 (Frontend)                          │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────┐  ┌─────────────┐  │
│  │ Bioreactor  │  │ SensorTrend  │  │ ControlPanel │  │ AlarmPanel  │  │
│  │  Profile    │  │   (趋势图)   │  │   (控制面板) │  │  (告警面板) │  │
│  │  (剖面图)   │  └───────┬──────┘  └───────┬──────┘  └──────┬──────┘  │
│  └───────┬─────┘          │                  │                  │         │
│          └─────────────────┴──────────────────┼──────────────────┘         │
│                                               │                            │
│                                       ┌───────▼───────┐                    │
│                                       │   Nginx       │                    │
│                                       │  (反向代理)   │                    │
│                                       └───────┬───────┘                    │
└───────────────────────────────────────────────┼────────────────────────────┘
                                                │
                                                │ HTTP/REST/WebSocket
                                                ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                              业务层 (Go Backend)                          │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                           Gin API Server                          │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐          │  │
│  │  │  sensors │  │ control  │  │  alarms  │  │  metrics │          │  │
│  │  │  (API)   │  │  (API)   │  │  (API)   │  │  (API)   │          │  │
│  │  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘          │  │
│  └───────┼──────────────┼──────────────┼──────────────┼────────────────┘  │
│          │              │              │              │                   │
│  ┌───────▼──────┐ ┌────▼──────┐ ┌─────▼─────┐ ┌────▼──────────┐        │
│  │ Sensor       │ │ Aeration  │ │ Carbon    │ │ Alarm        │        │
│  │ Collector    │ │ Controller│ │ Optimizer │ │ Router       │        │
│  │ (数据采集层) │ │ (控制层)  │ │ (优化层)  │ │ (告警层)    │        │
│  └───────┬──────┘ └────┬──────┘ └────┬──────┘ └──────┬───────┘        │
│          │             │              │                │                │
│          └─────────┬───┴──────────────┴────────┬───────┘                │
│                    │                           │                        │
│              ┌─────▼─────┐              ┌──────▼───────┐                │
│              │   InfluxDB│              │   MQTT Broker│                │
│              │ (时序数据库)│              │ (消息队列)   │                │
│              └─────▲─────┘              └──────▲───────┘                │
└────────────────────┼───────────────────────────────┼──────────────────────┘
                     │                               │
                     │                               │ MQTT QoS 1
                     │                               ▼
┌────────────────────┴────────────────────────────────────────────────────┐
│                            设备层 (Simulators)                           │
│  ┌──────────────────────┐                     ┌──────────────────────┐  │
│  │  DTU Sensor          │                     │   PLC Simulator      │  │
│  │  Simulator           │                     │                      │  │
│  │  - 75个传感器         │◄───────────────────►│  - 30段曝气控制      │  │
│  │  - 2分钟上报间隔     │   MQTT/HTTP         │  - 碳源投加控制      │  │
│  │  - 水质波动可配置    │                     │  - 指令执行反馈      │  │
│  └──────────────────────┘                     └──────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

### 模块说明

| 模块 | 职责 | 技术栈 |
|------|------|--------|
| [sensor_collector](backend/internal/collector/sensor_collector.go) | 传感器数据接收、校验、异常检测 | Go, Gin |
| [aeration_controller](backend/internal/controller/aeration_controller.go) | PID控制计算、指令下发 | Go, MQTT |
| [carbon_optimizer](backend/internal/controller/carbon_optimizer.go) | 碳源投加量优化、事件驱动 | Go, MQTT |
| [alarm_router](backend/internal/alarm/alarm_router.go) | 多通道告警推送、通道切换 | Go, WebSocket |
| [bioreactor_profile](frontend/js/bioreactor_profile.js) | 生化池剖面图绘制 | JavaScript, Canvas |
| [sensor_trend](frontend/js/sensor_trend.js) | 传感器趋势曲线 | JavaScript, Canvas |

## 目录结构

```
.
├── backend/
│   ├── cmd/
│   │   ├── server/           # 主服务入口
│   │   ├── dtu-simulator/    # DTU传感器模拟器
│   │   └── plc-simulator/    # PLC模拟器
│   ├── internal/
│   │   ├── collector/        # 数据采集模块
│   │   ├── controller/       # 控制和优化模块
│   │   ├── alarm/            # 告警模块
│   │   ├── api/              # API层
│   │   ├── config/           # 配置
│   │   ├── influxdb/         # 时序数据库客户端
│   │   ├── mqtt/             # MQTT客户端
│   │   ├── websocket/        # WebSocket服务
│   │   ├── messages/         # 消息定义
│   │   └── models/           # 数据模型
│   ├── pkg/
│   │   └── pid/              # PID控制器
│   ├── Dockerfile            # 多阶段构建Dockerfile
│   ├── config.yaml           # 服务配置
│   └── go.mod
├── frontend/                 # 前端代码
├── scripts/
│   └── influxdb/             # InfluxDB初始化脚本
├── config/
│   ├── influxdb/             # InfluxDB配置
│   ├── mosquitto/            # MQTT Broker配置
│   └── nginx/                # Nginx配置
├── docker-compose.yml        # 服务编排
├── .env                      # 环境变量
└── README.md
```

## 部署步骤

### 1. 环境要求

- Docker >= 20.10
- Docker Compose >= 1.29
- 至少4GB可用内存
- 至少20GB可用磁盘空间

### 2. 快速部署

```bash
# 1. 克隆项目
git clone <repository-url>
cd AI_solo_coder_task_A_031

# 2. 配置环境变量（可选，使用默认值可跳过）
cp .env.example .env
# 编辑 .env 文件

# 3. 启动所有服务
docker-compose up -d

# 4. 查看服务状态
docker-compose ps

# 5. 查看日志
docker-compose logs -f go-server

# 6. 停止服务
docker-compose down
```

### 3. 服务启动顺序

docker-compose已配置健康检查，服务将按以下顺序启动：

1. **InfluxDB** - 时序数据库（等待健康检查通过）
2. **MQTT Broker** - 消息队列（等待健康检查通过）
3. **Go Server** - 主服务（依赖InfluxDB和MQTT）
4. **DTU Simulator** - 传感器模拟器（依赖Go Server）
5. **PLC Simulator** - PLC模拟器（依赖MQTT）
6. **Frontend** - 前端（依赖Go Server）

### 4. 访问地址

| 服务 | 地址 | 说明 |
|------|------|------|
| 前端 | http://localhost | 主界面 |
| API | http://localhost/api/v1 | RESTful API |
| WebSocket | ws://localhost/ws | 实时数据推送 |
| pprof | http://localhost:6060/debug/pprof | 性能监控 |
| InfluxDB | http://localhost:8086 | 数据库管理 |
| MQTT | tcp://localhost:1883 | MQTT端口 |
| MQTT WS | http://localhost:9001 | MQTT WebSocket端口 |

## 模拟器配置说明

### DTU 传感器模拟器

#### 功能特性

- 模拟75个传感器的数据采集
- 支持2分钟默认上报间隔（可配置）
- 水质波动范围可配置
- 支持多种传感器类型：DO、NH3、NO3、PO4、COD、TN、TP、FLOW

#### 环境变量配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `API_URL` | http://go-server:8080/api/v1 | 后端API地址 |
| `REPORT_INTERVAL` | 120 | 上报间隔（秒） |
| `SENSOR_COUNT` | 75 | 传感器总数 |
| `WATER_QUALITY_FLUCTUATION` | 0.3 | 水质波动系数 (0-1) |

#### 传感器分布（75个）

| 类型 | 数量 | 工艺段 | 说明 |
|------|------|--------|------|
| DO | 30 | 好氧池 | 溶解氧传感器 |
| NH3 | 20 | 好氧池 | 氨氮传感器 |
| NO3 | 15 | 缺氧池 | 硝氮传感器 |
| PO4 | 5 | 厌氧池 | 磷酸盐传感器 |
| COD | 1 | 初沉池 | 进水COD |
| NH3 | 1 | 出水 | 出水氨氮 |
| TN | 1 | 出水 | 出水总氮 |
| TP | 1 | 出水 | 出水总磷 |
| FLOW | 1 | 粗格栅 | 进水流量 |

#### 水质波动说明

水质波动系数 `WATER_QUALITY_FLUCTUATION` 影响所有传感器的数值波动范围：

- **0.0**: 稳定水质，几乎无波动
- **0.3**: 正常波动（默认）
- **0.5**: 较大波动
- **1.0**: 剧烈波动

波动会影响：
- 噪声幅度
- 漂移速度
- 周期变化幅度
- 数值范围边界

### PLC 模拟器

#### 功能特性

- 模拟30段曝气控制
- 模拟碳源投加泵控制
- 接收MQTT控制指令
- 指令执行状态反馈（ACK）
- 定时上报设备状态
- 模拟设备故障和恢复

#### 环境变量配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `MQTT_BROKER` | tcp://mqtt-broker:1883 | MQTT Broker地址 |
| `MQTT_TOPIC_PREFIX` | sewage/ | MQTT主题前缀 |
| `STATUS_INTERVAL` | 15 | 状态上报间隔（秒） |
| `SIM_INTERVAL` | 1 | 模拟计算间隔（秒） |

#### MQTT 主题

| 主题 | QoS | 方向 | 说明 |
|------|-----|------|------|
| `sewage/control/aeration/#` | 1 | 接收 | 曝气控制指令 |
| `sewage/control/carbon` | 1 | 接收 | 碳源控制指令 |
| `sewage/plc/status/aeration/{section}` | 1 | 发布 | 曝气状态上报 |
| `sewage/plc/status/carbon` | 1 | 发布 | 碳源状态上报 |
| `sewage/plc/ack/{device_id}` | 1 | 发布 | 指令执行确认 |

#### 控制指令格式

**曝气控制指令**
```json
{
  "device_type": "aeration",
  "device_id": "aeration_1",
  "command": "set_air_flow",
  "params": {
    "section": 1,
    "air_flow": 150.5,
    "valve_open": 75.0
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

**碳源控制指令**
```json
{
  "device_type": "carbon_dosing",
  "device_id": "carbon_dosing_1",
  "command": "set_dosing",
  "params": {
    "dosing_rate": 50.0
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

#### 状态反馈格式

```json
{
  "device_id": "aeration_1",
  "device_type": "aeration",
  "status": "running",
  "actual_value": 148.3,
  "set_value": 150.5,
  "timestamp": "2024-01-01T00:00:00Z"
}
```

#### 指令确认格式

```json
{
  "command_id": "cmd_1_1704067200000000000",
  "device_id": "aeration_1",
  "status": "executed",
  "message": "Aeration command executed for section 1",
  "executed_at": "2024-01-01T00:00:00Z",
  "actual_value": 148.3
}
```

## 核心功能配置

### 性能监控 (pprof)

Go服务已开启pprof性能监控，默认监听6060端口。

**访问地址**: http://localhost:6060/debug/pprof/

**常用端点**:
- `/debug/pprof/heap` - 内存使用情况
- `/debug/pprof/goroutine` - Goroutine堆栈
- `/debug/pprof/cpu` - CPU使用情况
- `/debug/pprof/allocs` - 内存分配情况
- `/debug/pprof/block` - 阻塞操作情况
- `/debug/pprof/trace` - 执行追踪

**使用示例**:
```bash
# 查看CPU使用情况
go tool pprof http://localhost:6060/debug/pprof/cpu?seconds=30

# 查看内存使用情况
go tool pprof http://localhost:6060/debug/pprof/heap

# 查看Goroutine数量
curl http://localhost:6060/debug/pprof/goroutine?debug=1
```

**配置开关**:
```yaml
server:
  pprof:
    enabled: true  # 设置为false禁用
    port: 6060
```

### 数据保留和自动压缩

#### InfluxDB 数据保留策略

| 策略名称 | 保留时长 | 分片周期 | 用途 |
|----------|----------|----------|------|
| `one_year` | 365天 | 7天 | 默认策略，存储原始数据和1小时聚合 |
| `one_month` | 30天 | 7天 | 存储5分钟聚合数据 |
| `one_week` | 7天 | 1天 | 高频查询数据 |

#### 自动压缩配置

InfluxDB启用了Snappy压缩算法，配置文件位于 [influxdb.conf](config/influxdb/influxdb.conf):

```ini
[data]
  compression = "snappy"          # Snappy压缩算法
  compact-throughput = 48m        # 压缩吞吐量
  compact-throughput-burst = 48m  # 压缩吞吐量峰值
  cache-max-memory-size = "1g"    # 缓存最大内存
```

#### 连续查询（自动降采样）

| CQ名称 | 源数据 | 目标数据 | 聚合周期 |
|--------|--------|----------|----------|
| `cq_sensor_1h` | `sensor_data` | `one_year.sensor_data_1h` | 1小时 |
| `cq_sensor_5m` | `sensor_data` | `one_month.sensor_data_5m` | 5分钟 |
| `cq_aeration_1h` | `aeration_control` | `one_year.aeration_control_1h` | 1小时 |
| `cq_carbon_1h` | `carbon_dosing` | `one_year.carbon_dosing_1h` | 1小时 |
| `cq_metrics_1h` | `key_metrics` | `one_year.key_metrics_1h` | 1小时 |
| `cq_metrics_1d` | `key_metrics` | `one_year.key_metrics_1d` | 1天 |

### MQTT QoS 1 保证指令送达

系统所有MQTT通信使用QoS 1（至少一次），确保控制指令可靠送达。

#### QoS 1 工作机制

1. **发送方**发布消息，设置QoS=1
2. **Broker**收到消息后，返回PUBACK确认
3. **发送方**未收到确认时，自动重试
4. **Broker**持久化消息，直到收到确认

#### 配置位置

- **服务端配置**: [mosquitto.conf](config/mosquitto/mosquitto.conf)
  ```
  max_queued_messages 10000   # 最大消息队列
  persistent_client_expiration 1d  # 持久化客户端过期时间
  ```

- **客户端配置**: [config.yaml](backend/config.yaml)
  ```yaml
  mqtt:
    qos: 1  # 全局QoS设置
  ```

- **代码实现**: [mqtt.go](backend/internal/mqtt/mqtt.go)
  - 订阅: `client.Subscribe(topic, m.cfg.QoS, ...)`
  - 发布: `client.Publish(topic, m.cfg.QoS, false, data)`

## API 接口

### 传感器接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/sensors` | 获取所有传感器列表 |
| GET | `/api/v1/sensors/:id` | 获取单个传感器信息 |
| GET | `/api/v1/sensors/:id/trend` | 获取传感器趋势数据 |
| POST | `/api/v1/sensors/data` | 上报传感器数据 |

### 控制接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/control/aeration` | 获取所有曝气控制状态 |
| GET | `/api/v1/control/aeration/:section` | 获取单段曝气状态 |
| POST | `/api/v1/control/aeration/reset` | 重置PID控制器 |
| POST | `/api/v1/control/aeration/tuning` | 更新PID参数 |
| GET | `/api/v1/control/carbon` | 获取碳源控制状态 |
| POST | `/api/v1/control/carbon/config` | 更新碳源优化参数 |

### 告警接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/alarms/active` | 获取活跃告警 |
| POST | `/api/v1/alarms/:id/ack` | 确认告警 |

### 指标接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/metrics/latest` | 获取最新关键指标 |
| GET | `/api/v1/metrics/trend` | 获取指标趋势数据 |

## 常见问题

### 1. 服务启动后无法访问前端

检查80端口是否被占用：
```bash
netstat -ano | findstr :80
```

### 2. MQTT连接失败

检查MQTT Broker是否正常启动：
```bash
docker-compose logs mqtt-broker
```

### 3. InfluxDB写入失败

检查数据库是否初始化完成：
```bash
docker-compose exec influxdb influx -execute "SHOW DATABASES"
```

### 4. 传感器数据不上报

检查DTU模拟器日志：
```bash
docker-compose logs dtu-simulator
```

### 5. 调整传感器数量

修改 `.env` 文件中的 `DTU_SENSOR_COUNT`，然后重启：
```bash
docker-compose up -d --force-recreate dtu-simulator
```

### 6. 调整水质波动幅度

修改 `.env` 文件中的 `WATER_QUALITY_FLUCTUATION`：
```bash
WATER_QUALITY_FLUCTUATION=0.5  # 较大波动
docker-compose up -d --force-recreate dtu-simulator
```

### 7. 查看Go服务性能

通过pprof查看内存和CPU使用：
```bash
# 30秒CPU采样
go tool pprof http://localhost:6060/debug/pprof/cpu?seconds=30

# 内存采样
go tool pprof http://localhost:6060/debug/pprof/heap
```

## 开发指南

### 本地开发

```bash
# 1. 启动依赖服务
docker-compose up -d influxdb mqtt-broker

# 2. 编译Go服务
cd backend
go build -o server ./cmd/server

# 3. 运行Go服务
./server

# 4. 运行模拟器（新终端）
go run ./cmd/dtu-simulator
go run ./cmd/plc-simulator

# 5. 启动前端（新终端）
cd ../frontend
python -m http.server 8081
```

### 重新构建镜像

```bash
# 重新构建所有服务
docker-compose build

# 只构建Go服务
docker-compose build go-server

# 无缓存构建
docker-compose build --no-cache
```

## 许可证

MIT License
