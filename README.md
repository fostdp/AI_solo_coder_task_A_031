# 城市污水处理厂精确曝气与脱氮除磷优化系统

## 项目概述

本系统是一个完整的城市污水处理厂智能控制系统，针对日处理能力30万吨的污水处理厂设计。系统集成了实时数据采集、精确曝气控制、碳源投加优化、智能告警和数据可视化等功能，实现污水处理过程的精细化管理。

## 系统架构

```
┌─────────────────┐     MQTT      ┌─────────────────┐     HTTP/WebSocket     ┌─────────────────┐
│  4G DTU传感器   │ ─────────────> │   Go后端服务    │ ─────────────────────> │   前端Web应用   │
│  (75台传感器)   │               │                 │                        │   (Canvas可视化) │
└─────────────────┘               └────────┬────────┘                        └─────────────────┘
                                            │
                                            │ MQTT
                                            ▼
┌─────────────────┐               ┌─────────────────┐
│   PLC模拟器     │ <──────────── │    InfluxDB     │
│ (鼓风机/阀门)   │               │  (时序数据库)   │
└─────────────────┘               └─────────────────┘
```

## 功能特性

### 1. 数据采集与存储
- 75台传感器（30台DO、20台NH3、15台NO3、10台PO4）
- 每2分钟通过4G DTU上报数据
- InfluxDB时序数据库存储，支持长周期历史数据分析
- 数据保留策略：原始数据1年、小时聚合2年、日聚合5年

### 2. 精确曝气控制模型
- **控制算法**：PID + 前馈控制
- **控制目标**：
  - 好氧池末端氨氮：1-2mg/L
  - 溶解氧：1.5-2.5mg/L
- **控制输出**：各段曝气量设定值，通过MQTT下发到鼓风机和阀门

### 3. 碳源投加优化模型
- 基于缺氧池硝氮数据和进水COD计算最优碳源投加量
- 最大化总氮去除率的同时避免碳源浪费
- 实时动态调整，适应进水水质波动

### 4. 智能告警系统
- **两级告警机制**：
  - **一级告警（出水超标）**：出水氨氮>5mg/L 或 总氮>15mg/L，持续30分钟
  - **二级告警（设备故障）**：曝气风机故障 或 DO传感器离线
- **通知方式**：短信 + WebSocket 双通道推送

### 5. 数据可视化
- Canvas绘制全厂工艺平面图
- 生化池剖面图，展示DO沿深度分布
- 传感器用圆点标注，颜色表示偏离程度：
  - 绿色：偏离<10%
  - 黄色：偏离10%-20%
  - 红色：偏离>20%
- 点击传感器显示近6小时趋势曲线
- KPI趋势展示：吨水电耗、碳源单耗、去除率

## 项目结构

```
AI_solo_coder_task_A_031/
├── backend/                    # Go后端服务
│   ├── cmd/server/
│   │   └── main.go            # 主程序入口
│   ├── pkg/
│   │   ├── models/            # 数据模型
│   │   │   ├── models.go
│   │   │   └── sensor_config.go
│   │   ├── influxdb/          # InfluxDB客户端
│   │   │   └── client.go
│   │   ├── control/           # 控制算法
│   │   │   ├── aeration.go    # 精确曝气控制
│   │   │   └── carbon.go      # 碳源投加优化
│   │   ├── mqtt/              # MQTT客户端
│   │   │   └── client.go
│   │   ├── websocket/         # WebSocket服务
│   │   │   └── server.go
│   │   └── alert/             # 告警管理
│   │       └── manager.go
│   ├── config.yaml            # 配置文件
│   └── go.mod                 # Go模块依赖
├── frontend/                   # 前端Web应用
│   ├── index.html             # 主页面
│   ├── css/
│   │   └── style.css          # 样式文件
│   └── js/
│       ├── config.js          # 前端配置
│       ├── canvas.js          # Canvas绘制
│       ├── charts.js          # 图表绘制
│       ├── websocket.js       # WebSocket客户端
│       └── main.js            # 主逻辑
├── influxdb/                   # InfluxDB初始化
│   └── init.iql               # 数据库初始化脚本
├── simulators/                 # 模拟器
│   ├── dtu_sensor_simulator.py   # DTU传感器模拟器
│   └── plc_simulator.py          # PLC设备模拟器
└── README.md                  # 项目说明文档
```

## 技术栈

### 后端
- **语言**：Go 1.21+
- **Web框架**：Gin
- **数据库**：InfluxDB 1.8+
- **消息队列**：MQTT (Eclipse Paho)
- **实时通信**：WebSocket (gorilla/websocket)
- **配置管理**：Viper

### 前端
- **语言**：JavaScript (ES6+)
- **可视化**：HTML5 Canvas
- **通信**：WebSocket + Fetch API
- **样式**：CSS3 (响应式设计)

### 模拟器
- **语言**：Python 3.8+
- **MQTT客户端**：paho-mqtt

## 快速开始

### 环境要求
- Go 1.21+
- Node.js 14+ (可选，用于启动前端服务器)
- Python 3.8+ (用于运行模拟器)
- InfluxDB 1.8+
- MQTT Broker (如 Eclipse Mosquitto)

### 1. 启动基础设施

#### 启动InfluxDB
```bash
# 进入InfluxDB安装目录，启动服务
influxd -config influxdb.conf

# 执行初始化脚本
influx -import -path=influxdb/init.iql -username=admin -password=admin123
```

#### 启动MQTT Broker (Mosquitto)
```bash
# Windows
mosquitto -c mosquitto.conf

# 或使用Docker
docker run -d -p 1883:1883 -p 9001:9001 eclipse-mosquitto
```

### 2. 启动Go后端服务

```bash
cd backend

# 下载依赖
go mod download

# 编译运行
go run cmd/server/main.go
```

服务启动后访问：`http://localhost:8080`

### 3. 启动模拟器

#### 启动DTU传感器模拟器
```bash
cd simulators
pip install paho-mqtt
python dtu_sensor_simulator.py
```

#### 启动PLC设备模拟器
```bash
cd simulators
python plc_simulator.py
```

### 4. 启动前端应用

由于浏览器安全策略，建议使用本地HTTP服务器访问前端：

#### 方式一：使用Python启动
```bash
cd frontend
python -m http.server 8000
```

#### 方式二：使用Node.js启动
```bash
cd frontend
npx http-server -p 8000
```

然后访问：`http://localhost:8000`

## API接口说明

### RESTful API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/sensors` | 获取传感器列表 |
| GET | `/api/sensors/:id` | 获取单个传感器详情 |
| GET | `/api/sensors/:id/trend` | 获取传感器趋势数据 |
| GET | `/api/trend/:type` | 获取某类型传感器趋势 |
| GET | `/api/alerts` | 获取告警列表 |
| POST | `/api/alerts/:id/ack` | 确认告警 |
| GET | `/api/kpi/current` | 获取当前KPI数据 |
| GET | `/api/kpi/history` | 获取历史KPI数据 |
| GET | `/api/system/status` | 获取系统状态 |
| GET | `/api/biological-profile` | 获取生化池剖面数据 |
| POST | `/api/control/aeration` | 手动调整曝气设定值 |
| POST | `/api/control/carbon` | 手动调整碳源投加量 |

### WebSocket消息格式

#### 服务端推送消息

```javascript
// 传感器数据
{
    "type": "sensor_data",
    "data": {
        "sensor_id": "DO-001",
        "type": "DO",
        "value": 2.15,
        "timestamp": "2024-01-01T12:00:00Z",
        "location": "aerobic1"
    }
}

// 告警信息
{
    "type": "alert",
    "data": {
        "id": 123,
        "level": 1,
        "message": "出水氨氮超过5mg/L",
        "sensor_id": "NH3-020",
        "timestamp": "2024-01-01T12:00:00Z",
        "acknowledged": false
    }
}

// KPI更新
{
    "type": "kpi_update",
    "data": {
        "energy_per_ton": 0.32,
        "carbon_per_ton": 0.23,
        "tn_removal_rate": 82.5
    }
}

// 控制指令
{
    "type": "control_command",
    "data": {
        "target": "aerobic1_blower",
        "value": 75.5,
        "timestamp": "2024-01-01T12:00:00Z"
    }
}
```

### MQTT主题说明

| 主题 | 方向 | 说明 |
|------|------|------|
| `sensors/data` | 传感器 → 后端 | 传感器数据上报 |
| `sensors/status` | 传感器 → 后端 | 传感器状态上报 |
| `control/blower/+/setpoint` | 后端 → PLC | 鼓风机设定值下发 |
| `control/valve/+/setpoint` | 后端 → PLC | 阀门开度设定值下发 |
| `control/carbon/setpoint` | 后端 → PLC | 碳源投加量下发 |
| `plc/status/blower/+` | PLC → 后端 | 鼓风机状态上报 |
| `plc/status/valve/+` | PLC → 后端 | 阀门状态上报 |

## 控制算法说明

### 精确曝气控制 (PID+前馈)

```
曝气量设定值 = PID输出 + 前馈补偿

PID输出 = Kp*(e(t) + 1/Ti*∫e(t)dt + Td*de(t)/dt)
其中 e(t) = DO设定值 - DO实测值

前馈补偿 = f(进水氨氮浓度, 进水流量, 污泥浓度)
```

控制参数可在 `backend/config.yaml` 中调整：

```yaml
aeration_control:
  do_setpoint: 2.0           # DO设定值 (mg/L)
  nh3_setpoint: 1.5          # 氨氮设定值 (mg/L)
  do_low_limit: 1.5          # DO下限 (mg/L)
  do_high_limit: 2.5         # DO上限 (mg/L)
  kp: 0.5                    # 比例系数
  ki: 0.1                    # 积分系数
  kd: 0.05                   # 微分系数
  feedforward_gain: 0.3      # 前馈增益
```

### 碳源投加优化模型

```
碳源投加量 = (硝氮浓度 - 目标硝氮浓度) * 体积 * 碳氮比 / 碳源有效含量

其中：
- 碳氮比 = 5:1 (甲醇) 或 4:1 (乙酸钠)
- 根据进水COD动态调整投加策略
```

## 告警配置

告警阈值可在配置文件中调整：

```yaml
alert:
  level1:
    nh3_threshold: 5.0       # 氨氮阈值 (mg/L)
    tn_threshold: 15.0       # 总氮阈值 (mg/L)
    duration_minutes: 30     # 持续时间
  level2:
    blower_fault: true       # 鼓风机故障
    sensor_offline: true     # 传感器离线
    offline_minutes: 5       # 离线判定时间
  notification:
    sms: true                # 启用短信通知
    websocket: true          # 启用WebSocket通知
    sms_gateway_url: ""      # 短信网关地址
    admin_phones: []         # 管理员手机号
```

## 前端功能说明

### 工艺平面图
- 展示全厂各工艺段布局
- 水流方向动态显示
- 硝化液回流、污泥回流管道标注
- 支持缩放、平移、传感器筛选

### 生化池剖面图
- 展示厌氧-缺氧-好氧各池体
- DO浓度沿深度分布热力图
- 气泡动画模拟曝气效果
- 搅拌状态显示

### 趋势曲线
- DO、NH3、NO3、PO4四种参数趋势
- 支持6小时、24小时、7天时间范围
- 实时更新，平滑曲线显示
- 显示最新值、平均值统计

### KPI详情
- 吨水电耗趋势
- 碳源单耗趋势
- 总氮去除率趋势
- 综合水质评分仪表盘
- 各污染物去除率占比

## 模拟器说明

### DTU传感器模拟器 (`dtu_sensor_simulator.py`)

模拟75台传感器的真实运行：
- DO传感器：正常值范围1.5-2.5mg/L
- NH3传感器：正常值范围0.5-5mg/L
- NO3传感器：正常值范围5-20mg/L
- PO4传感器：正常值范围0.2-2mg/L
- 每2分钟上报一次数据
- 支持随机异常数据注入（测试告警）

### PLC设备模拟器 (`plc_simulator.py`)

模拟以下设备的运行状态：
- 4台鼓风机：转速、电流、风压、运行状态
- 20个阀门：开度、反馈、在线状态
- 3台碳源投加泵：频率、流量、运行状态

支持接收控制指令并调整输出，模拟真实PLC行为。

## 监控指标

系统内置以下监控指标，可通过Prometheus采集：

| 指标名称 | 说明 | 单位 |
|----------|------|------|
| `sewage_plant_energy_per_ton` | 吨水电耗 | kWh/吨 |
| `sewage_plant_carbon_per_ton` | 碳源单耗 | kg/吨 |
| `sewage_plant_tn_removal_rate` | 总氮去除率 | % |
| `sewage_plant_nh3_removal_rate` | 氨氮去除率 | % |
| `sewage_plant_tp_removal_rate` | 总磷去除率 | % |
| `sewage_plant_effluent_nh3` | 出水氨氮 | mg/L |
| `sewage_plant_effluent_tn` | 出水总氮 | mg/L |
| `sewage_plant_sensor_online_rate` | 传感器在线率 | % |
| `sewage_plant_alert_count` | 告警次数 | 次 |
| `sewage_plant_blower_efficiency` | 鼓风机效率 | % |

## 常见问题

### Q: 后端启动时连接InfluxDB失败？
A: 请检查InfluxDB服务是否启动，以及`config.yaml`中的连接配置是否正确。

### Q: 前端无法连接WebSocket？
A: 请检查后端服务是否正常启动，以及前端`config.js`中的`WS_URL`配置是否正确。

### Q: 模拟器发送的数据前端看不到？
A: 请检查MQTT Broker是否正常运行，模拟器和后端是否连接到同一个Broker。

### Q: 如何调整控制算法参数？
A: 编辑`backend/config.yaml`中的相关参数，重启后端服务生效。

### Q: 如何添加新的传感器？
A: 在`backend/pkg/models/sensor_config.go`中添加传感器配置，并更新数据库。

## 性能优化建议

1. **InfluxDB优化**：
   - 使用索引优化查询性能
   - 定期运行压缩策略
   - 考虑使用InfluxDB集群应对高并发

2. **后端优化**：
   - 增加Goroutine池大小
   - 启用Gzip压缩
   - 使用连接池管理数据库连接

3. **前端优化**：
   - 启用Canvas离屏渲染
   - 实现数据虚拟滚动
   - 使用Web Worker处理复杂计算

## 扩展功能建议

1. **历史数据分析**：添加机器学习模型预测水质趋势
2. **移动端适配**：开发移动端小程序或APP
3. **多厂管理**：支持多污水处理厂集中管理
4. **报表系统**：自动生成日报、周报、月报
5. **视频监控集成**：接入厂区摄像头画面
6. **能耗分析**：详细的能耗统计和分析功能

## 许可证

MIT License

## 联系方式

如有问题或建议，请联系项目维护团队。
