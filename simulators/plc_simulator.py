#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
PLC 模拟器
模拟污水处理厂的鼓风机、阀门等设备的PLC控制逻辑
- 接收MQTT控制指令
- 模拟设备状态变化
- 上报设备状态
"""

import json
import time
import random
import threading
import math
from datetime import datetime
import paho.mqtt.client as mqtt

MQTT_BROKER = "localhost"
MQTT_PORT = 1883
CONTROL_TOPIC = "control/+/+"
STATUS_TOPIC = "plc/{plc_id}/status"
REPORT_INTERVAL = 5

PLC_CONFIGS = [
    {
        "plc_id": "PLC-AER-01",
        "name": "曝气系统PLC",
        "devices": [
            {"type": "fan", "id": "aerobic1_fan", "name": "1号段曝气风机", "max_speed": 100, "min_speed": 30},
            {"type": "fan", "id": "aerobic2_fan", "name": "2号段曝气风机", "max_speed": 100, "min_speed": 30},
            {"type": "fan", "id": "aerobic3_fan", "name": "3号段曝气风机", "max_speed": 100, "min_speed": 30},
            {"type": "valve", "id": "aerobic1_valve", "name": "1号段进气阀门", "max_opening": 100, "min_opening": 0},
            {"type": "valve", "id": "aerobic2_valve", "name": "2号段进气阀门", "max_opening": 100, "min_opening": 0},
            {"type": "valve", "id": "aerobic3_valve", "name": "3号段进气阀门", "max_opening": 100, "min_opening": 0},
        ]
    },
    {
        "plc_id": "PLC-CARB-01",
        "name": "碳源投加PLC",
        "devices": [
            {"type": "pump", "id": "carbon_pump_01", "name": "碳源投加泵1号", "max_dosage": 50, "min_dosage": 0},
            {"type": "valve", "id": "carbon_valve_01", "name": "碳源阀门1号", "max_opening": 100, "min_opening": 0},
            {"type": "flow_meter", "id": "carbon_flow_01", "name": "碳源流量计", "max_flow": 100, "min_flow": 0},
        ]
    },
    {
        "plc_id": "PLC-MIX-01",
        "name": "混合液回流PLC",
        "devices": [
            {"type": "pump", "id": "mixed_pump_01", "name": "混合液回流泵", "max_flow": 1500, "min_flow": 0},
            {"type": "valve", "id": "mixed_valve_01", "name": "回流阀门", "max_opening": 100, "min_opening": 0},
        ]
    },
    {
        "plc_id": "PLC-SLU-01",
        "name": "污泥回流PLC",
        "devices": [
            {"type": "pump", "id": "sludge_pump_01", "name": "污泥回流泵1号", "max_flow": 500, "min_flow": 0},
            {"type": "pump", "id": "sludge_pump_02", "name": "污泥回流泵2号", "max_flow": 500, "min_flow": 0},
        ]
    }
]


class PLCDevice:
    def __init__(self, device_config, plc_id):
        self.type = device_config["type"]
        self.id = device_config["id"]
        self.name = device_config["name"]
        self.plc_id = plc_id
        self.max_value = device_config.get(f"max_{self._get_value_key()}", 100)
        self.min_value = device_config.get(f"min_{self._get_value_key()}", 0)

        self.current_value = 50.0
        self.target_value = 50.0
        self.status = "running"
        self.fault_code = ""
        self.last_command = None
        self.last_command_time = None
        self.start_time = datetime.now()
        self.run_hours = 0
        self.efficiency = 0.95
        self.vibration = 0.0
        self.temperature = 25.0
        self.current_draw = 0.0
        self.pressure = 0.0
        self.flow_rate = 0.0

        self.fault_probability = 0.0001
        self.response_time = 0.5

    def _get_value_key(self):
        if self.type == "fan":
            return "speed"
        elif self.type == "valve":
            return "opening"
        elif self.type == "pump":
            return "dosage" if "carbon" in self.id else "flow"
        else:
            return "value"

    def _get_value_unit(self):
        if self.type == "fan":
            return "%"
        elif self.type == "valve":
            return "%"
        elif self.type == "pump":
            return "kg/h" if "carbon" in self.id else "m3/h"
        elif self.type == "flow_meter":
            return "m3/h"
        else:
            return ""

    def execute_command(self, action, value):
        self.last_command = action
        self.last_command_time = datetime.now()

        if action in ["set_speed", "set_opening", "set_dosage", "set_flow"]:
            self.target_value = max(self.min_value, min(self.max_value, value))
            threading.Timer(self.response_time, self._apply_value).start()
            return True
        elif action == "start":
            self.status = "running"
            return True
        elif action == "stop":
            self.status = "stopped"
            self.target_value = 0
            return True
        elif action == "reset":
            self.status = "running"
            self.fault_code = ""
            return True
        return False

    def _apply_value(self):
        self.current_value = self.target_value
        self._update_parameters()

    def _update_parameters(self):
        if self.status == "running":
            load_factor = self.current_value / self.max_value if self.max_value > 0 else 0

            if self.type == "fan":
                self.current_draw = 50 + load_factor * 150
                self.vibration = 0.5 + load_factor * 2.5
                self.temperature = 25 + load_factor * 30
                self.pressure = 0.1 + load_factor * 0.5
                self.flow_rate = load_factor * 5000
            elif self.type == "pump":
                self.current_draw = 20 + load_factor * 80
                self.vibration = 0.3 + load_factor * 1.5
                self.temperature = 25 + load_factor * 20
                self.flow_rate = load_factor * self.max_value
            elif self.type == "valve":
                self.flow_rate = load_factor * 1000
        else:
            self.current_draw = 0
            self.vibration = 0
            self.temperature = 25
            self.flow_rate = 0

        if random.random() < self.fault_probability:
            self._generate_fault()

    def _generate_fault(self):
        fault_types = [
            ("E001", "过载保护"),
            ("E002", "温度过高"),
            ("E003", "振动异常"),
            ("E004", "通信故障"),
            ("E005", "电源异常"),
        ]
        fault_code, fault_msg = random.choice(fault_types)
        self.fault_code = fault_code
        self.status = "fault"
        print(f"[{self.plc_id}] 设备 {self.id} 故障: {fault_msg} ({fault_code})")

        threading.Timer(60, self._clear_fault).start()

    def _clear_fault(self):
        if self.status == "fault":
            self.fault_code = ""
            self.status = "running"
            print(f"[{self.plc_id}] 设备 {self.id} 故障已清除")

    def update(self):
        time_factor = math.sin(time.time() / 60) * 0.05

        if self.status == "running" and abs(self.current_value - self.target_value) > 0.1:
            step = (self.target_value - self.current_value) * 0.1
            self.current_value += step
            self._update_parameters()

        self.run_hours += 5 / 3600

        if self.status == "running":
            self.vibration += random.uniform(-0.1, 0.1)
            self.temperature += random.uniform(-0.5, 0.5)
            self.current_draw += random.uniform(-2, 2)

            self.vibration = max(0, min(5, self.vibration))
            self.temperature = max(20, min(80, self.temperature))
            self.current_draw = max(0, self.current_draw)

    def get_status(self):
        return {
            "plc_id": self.plc_id,
            "device_type": self.type,
            "device_id": self.id,
            "device_name": self.name,
            "status": self.status,
            "value": round(self.current_value, 2),
            "target_value": round(self.target_value, 2),
            "unit": self._get_value_unit(),
            "fault_code": self.fault_code,
            "timestamp": datetime.now().isoformat(),
            "parameters": {
                "run_hours": round(self.run_hours, 2),
                "efficiency": round(self.efficiency, 3),
                "vibration": round(self.vibration, 2),
                "temperature": round(self.temperature, 1),
                "current_draw": round(self.current_draw, 1),
                "pressure": round(self.pressure, 3),
                "flow_rate": round(self.flow_rate, 2),
            },
            "last_command": self.last_command,
            "last_command_time": self.last_command_time.isoformat() if self.last_command_time else None,
        }


class PLCSimulator:
    def __init__(self, plc_config):
        self.plc_id = plc_config["plc_id"]
        self.name = plc_config["name"]
        self.devices = {}

        for dev_config in plc_config["devices"]:
            device = PLCDevice(dev_config, self.plc_id)
            self.devices[device.id] = device

        self.client = None
        self.connected = False
        self.scan_cycle = 100
        self.communication_errors = 0
        self.last_scan = None

    def connect_mqtt(self):
        self.client = mqtt.Client(client_id=self.plc_id, clean_session=True)
        self.client.username_pw_set("admin", "admin123")
        self.client.on_connect = self.on_connect
        self.client.on_disconnect = self.on_disconnect
        self.client.on_message = self.on_message

        try:
            self.client.connect(MQTT_BROKER, MQTT_PORT, 60)
            self.client.loop_start()
            return True
        except Exception as e:
            print(f"[{self.plc_id}] MQTT连接失败: {e}")
            return False

    def on_connect(self, client, userdata, flags, rc):
        if rc == 0:
            self.connected = True
            print(f"[{self.plc_id}] MQTT连接成功")
            self.client.subscribe(CONTROL_TOPIC, qos=1)
        else:
            print(f"[{self.plc_id}] MQTT连接失败，错误码: {rc}")

    def on_disconnect(self, client, userdata, rc):
        self.connected = False
        print(f"[{self.plc_id}] MQTT断开连接，错误码: {rc}")

    def on_message(self, client, userdata, msg):
        try:
            topic_parts = msg.topic.split("/")
            if len(topic_parts) >= 3 and topic_parts[0] == "control":
                target_type = topic_parts[1]
                target_id = topic_parts[2]

                payload = json.loads(msg.payload.decode())
                action = payload.get("action")
                value = payload.get("value", 0)

                device = self.devices.get(target_id)
                if device:
                    success = device.execute_command(action, value)
                    if success:
                        print(f"[{self.plc_id}] 执行命令: {target_id} {action}={value}")
                    else:
                        print(f"[{self.plc_id}] 命令执行失败: {target_id} {action}")
                else:
                    for dev in self.devices.values():
                        if dev.type == target_type:
                            success = dev.execute_command(action, value)
                            if success:
                                print(f"[{self.plc_id}] 执行命令: {dev.id} {action}={value}")

        except Exception as e:
            print(f"[{self.plc_id}] 处理命令异常: {e}")
            self.communication_errors += 1

    def update_devices(self):
        self.last_scan = datetime.now()
        for device in self.devices.values():
            device.update()

    def report_status(self):
        if not self.connected:
            if not self.connect_mqtt():
                return

        for device in self.devices.values():
            status = device.get_status()
            topic = STATUS_TOPIC.format(plc_id=self.plc_id)

            try:
                payload = json.dumps(status, ensure_ascii=False)
                self.client.publish(topic, payload, qos=1)
            except Exception as e:
                print(f"[{self.plc_id}] 上报状态失败: {e}")
                self.communication_errors += 1

    def get_summary(self):
        running = sum(1 for d in self.devices.values() if d.status == "running")
        fault = sum(1 for d in self.devices.values() if d.status == "fault")
        return {
            "plc_id": self.plc_id,
            "name": self.name,
            "connected": self.connected,
            "total_devices": len(self.devices),
            "running_devices": running,
            "fault_devices": fault,
            "communication_errors": self.communication_errors,
            "last_scan": self.last_scan.isoformat() if self.last_scan else None,
        }

    def disconnect(self):
        if self.client:
            self.client.loop_stop()
            self.client.disconnect()
        self.connected = False


def run_simulation():
    print("=" * 60)
    print("PLC 模拟器启动")
    print(f"MQTT Broker: {MQTT_BROKER}:{MQTT_PORT}")
    print(f"上报间隔: {REPORT_INTERVAL}秒")
    print("=" * 60)

    plcs = []
    for config in PLC_CONFIGS:
        plc = PLCSimulator(config)
        plcs.append(plc)

    print(f"\n创建了 {len(plcs)} 个PLC:")
    for plc in plcs:
        dev_types = {}
        for dev in plc.devices.values():
            dev_types[dev.type] = dev_types.get(dev.type, 0) + 1
        dev_str = ", ".join([f"{t}:{c}个" for t, c in dev_types.items()])
        print(f"  {plc.plc_id} ({plc.name}): {dev_str}")

    print("\n连接MQTT...")
    for plc in plcs:
        plc.connect_mqtt()
        time.sleep(0.1)

    print("\n开始运行...")
    print("=" * 60)

    last_report = time.time()
    scan_count = 0

    try:
        while True:
            for plc in plcs:
                plc.update_devices()

            scan_count += 1

            if time.time() - last_report >= REPORT_INTERVAL:
                threads = []
                for plc in plcs:
                    t = threading.Thread(target=plc.report_status)
                    threads.append(t)
                    t.start()

                for t in threads:
                    t.join()

                last_report = time.time()

                if scan_count % 12 == 0:
                    print(f"\n[{datetime.now().strftime('%H:%M:%S')}] 运行状态:")
                    for plc in plcs:
                        summary = plc.get_summary()
                        print(f"  {summary['plc_id']}: 运行{summary['running_devices']}/{summary['total_devices']} "
                              f"故障{summary['fault_devices']} "
                              f"通信错误{summary['communication_errors']}")

            time.sleep(0.1)

    except KeyboardInterrupt:
        print("\n\n收到停止信号，正在关闭...")
        for plc in plcs:
            plc.disconnect()
        print("PLC模拟器已停止")


def run_single_plc_test():
    print("单PLC测试模式")
    print("=" * 60)

    plc = PLCSimulator(PLC_CONFIGS[0])
    plc.connect_mqtt()

    try:
        for i in range(20):
            plc.update_devices()
            if i % 5 == 0:
                plc.report_status()
                print(json.dumps(plc.get_summary(), indent=2, ensure_ascii=False))
            time.sleep(1)
    except KeyboardInterrupt:
        pass
    finally:
        plc.disconnect()


if __name__ == "__main__":
    import sys

    if len(sys.argv) > 1 and sys.argv[1] == "test":
        run_single_plc_test()
    else:
        run_simulation()
