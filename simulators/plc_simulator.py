#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
PLC 模拟器 (增强版)
支持环境变量配置、MQTT QoS 1、指令执行反馈、设备状态上报
"""

import json
import time
import random
import threading
import math
import os
from datetime import datetime
import paho.mqtt.client as mqtt


def get_env_bool(key, default=False):
    val = os.environ.get(key, str(default)).lower()
    return val in ('true', '1', 'yes', 'y')


def get_env_float(key, default):
    try:
        return float(os.environ.get(key, default))
    except (ValueError, TypeError):
        return default


def get_env_int(key, default):
    try:
        return int(os.environ.get(key, default))
    except (ValueError, TypeError):
        return default


MQTT_BROKER = os.environ.get('MQTT_BROKER', 'localhost')
MQTT_PORT = get_env_int('MQTT_PORT', 1883)
MQTT_USERNAME = os.environ.get('MQTT_USERNAME', 'admin')
MQTT_PASSWORD = os.environ.get('MQTT_PASSWORD', 'admin123')
MQTT_QOS = get_env_int('MQTT_QOS', 1)
CONTROL_TOPIC = os.environ.get('CONTROL_TOPIC', 'control/+/+')
STATUS_TOPIC = os.environ.get('STATUS_TOPIC', 'plc/{plc_id}/status')
FEEDBACK_TOPIC = os.environ.get('FEEDBACK_TOPIC', 'plc/{plc_id}/feedback')
REPORT_INTERVAL = get_env_int('REPORT_INTERVAL', 5)
STATUS_TOPIC_QOS = get_env_int('STATUS_TOPIC_QOS', 1)

FAULT_SIMULATION_ENABLED = get_env_bool('FAULT_SIMULATION_ENABLED', True)
RESPONSE_TIME = get_env_float('RESPONSE_TIME', 0.5)

PLC_CONFIGS = [
    {
        "plc_id": os.environ.get('PLC_AER_ID', 'PLC-AER-01'),
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
        "plc_id": os.environ.get('PLC_CARB_ID', 'PLC-CARB-01'),
        "name": "碳源投加PLC",
        "devices": [
            {"type": "pump", "id": "carbon_pump_01", "name": "碳源投加泵1号", "max_dosage": 100, "min_dosage": 0},
            {"type": "pump", "id": "carbon_pump_02", "name": "碳源投加泵2号", "max_dosage": 100, "min_dosage": 0},
            {"type": "valve", "id": "carbon_valve_01", "name": "碳源阀门1号", "max_opening": 100, "min_opening": 0},
            {"type": "valve", "id": "carbon_valve_02", "name": "碳源阀门2号", "max_opening": 100, "min_opening": 0},
            {"type": "flow_meter", "id": "carbon_flow_01", "name": "碳源流量计", "max_flow": 200, "min_flow": 0},
        ]
    },
    {
        "plc_id": os.environ.get('PLC_MIX_ID', 'PLC-MIX-01'),
        "name": "混合液回流PLC",
        "devices": [
            {"type": "pump", "id": "mixed_pump_01", "name": "混合液回流泵", "max_flow": 2000, "min_flow": 0},
            {"type": "pump", "id": "mixed_pump_02", "name": "混合液回流泵备用", "max_flow": 2000, "min_flow": 0},
            {"type": "valve", "id": "mixed_valve_01", "name": "回流阀门", "max_opening": 100, "min_opening": 0},
        ]
    },
    {
        "plc_id": os.environ.get('PLC_SLU_ID', 'PLC-SLU-01'),
        "name": "污泥回流PLC",
        "devices": [
            {"type": "pump", "id": "sludge_pump_01", "name": "污泥回流泵1号", "max_flow": 800, "min_flow": 0},
            {"type": "pump", "id": "sludge_pump_02", "name": "污泥回流泵2号", "max_flow": 800, "min_flow": 0},
            {"type": "pump", "id": "sludge_pump_03", "name": "剩余污泥泵", "max_flow": 300, "min_flow": 0},
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
        self.last_command_result = None
        self.start_time = datetime.now()
        self.run_hours = 0
        self.efficiency = 0.95
        self.vibration = 0.0
        self.temperature = 25.0
        self.current_draw = 0.0
        self.pressure = 0.0
        self.flow_rate = 0.0

        self.fault_probability = 0.0001
        self.response_time = RESPONSE_TIME
        self.command_count = 0
        self.success_count = 0
        self.failed_count = 0

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
        self.command_count += 1

        success = False
        result_msg = ""

        if action in ["set_speed", "set_opening", "set_dosage", "set_flow"]:
            clamped_value = max(self.min_value, min(self.max_value, value))
            if clamped_value != value:
                result_msg = f"值已限制在[{self.min_value}, {self.max_value}]范围内"
            self.target_value = clamped_value
            threading.Timer(self.response_time, self._apply_value).start()
            success = True
            self.success_count += 1
            result_msg = result_msg or f"已设置目标值: {clamped_value}"
        elif action == "start":
            self.status = "running"
            success = True
            self.success_count += 1
            result_msg = "设备已启动"
        elif action == "stop":
            self.status = "stopped"
            self.target_value = 0
            self._apply_value()
            success = True
            self.success_count += 1
            result_msg = "设备已停止"
        elif action == "reset":
            self.status = "running"
            self.fault_code = ""
            self.failed_count = max(0, self.failed_count - 1)
            success = True
            self.success_count += 1
            result_msg = "设备已复位"
        elif action == "emergency_stop":
            self.status = "stopped"
            self.target_value = 0
            self.current_value = 0
            self._update_parameters()
            success = True
            self.success_count += 1
            result_msg = "紧急停止已执行"
        else:
            success = False
            self.failed_count += 1
            result_msg = f"未知命令: {action}"

        self.last_command_result = {
            "action": action,
            "requested_value": value,
            "target_value": self.target_value,
            "success": success,
            "message": result_msg,
            "timestamp": self.last_command_time.isoformat(),
        }

        print(f"[{self.plc_id}] {self.id}: {action}={value} -> {'成功' if success else '失败'}: {result_msg}")

        return self.last_command_result

    def _apply_value(self):
        self.current_value = self.target_value
        self._update_parameters()

    def _update_parameters(self):
        if self.status == "running" and self.current_value > 0:
            load_factor = self.current_value / self.max_value if self.max_value > 0 else 0

            if self.type == "fan":
                self.current_draw = 50 + load_factor * 150
                self.vibration = 0.5 + load_factor * 2.5
                self.temperature = 25 + load_factor * 30
                self.pressure = 0.1 + load_factor * 0.5
                self.flow_rate = load_factor * 6000
            elif self.type == "pump":
                self.current_draw = 20 + load_factor * 80
                self.vibration = 0.3 + load_factor * 1.5
                self.temperature = 25 + load_factor * 20
                self.flow_rate = load_factor * self.max_value
            elif self.type == "valve":
                self.current_draw = 5 + load_factor * 10
                self.flow_rate = load_factor * 1500
        else:
            self.current_draw = 0
            self.vibration = 0
            self.temperature = 25
            self.flow_rate = 0

        if FAULT_SIMULATION_ENABLED and random.random() < self.fault_probability:
            self._generate_fault()

    def _generate_fault(self):
        fault_types = [
            ("E001", "过载保护"),
            ("E002", "温度过高"),
            ("E003", "振动异常"),
            ("E004", "通信故障"),
            ("E005", "电源异常"),
            ("E006", "执行器卡涩"),
        ]
        fault_code, fault_msg = random.choice(fault_types)
        self.fault_code = fault_code
        self.status = "fault"
        self.failed_count += 1
        print(f"[{self.plc_id}] 设备 {self.id} 故障: {fault_msg} ({fault_code})")

        threading.Timer(random.randint(30, 120), self._clear_fault).start()

    def _clear_fault(self):
        if self.status == "fault":
            self.fault_code = ""
            self.status = "running"
            print(f"[{self.plc_id}] 设备 {self.id} 故障已自动清除")

    def update(self):
        time_factor = math.sin(time.time() / 60) * 0.03

        if self.status == "running" and abs(self.current_value - self.target_value) > 0.1:
            step = (self.target_value - self.current_value) * 0.15
            self.current_value += step
            self._update_parameters()

        self.run_hours += REPORT_INTERVAL / 3600

        if self.status == "running":
            self.vibration += random.uniform(-0.05, 0.05)
            self.temperature += random.uniform(-0.3, 0.3)
            self.current_draw += random.uniform(-1, 1)

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
            "statistics": {
                "command_count": self.command_count,
                "success_count": self.success_count,
                "failed_count": self.failed_count,
            },
            "last_command": self.last_command_result,
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
        self.commands_received = 0
        self.last_scan = None

    def connect_mqtt(self):
        client_id = f"{self.plc_id}_{int(time.time())}"
        self.client = mqtt.Client(client_id=client_id, clean_session=True)
        self.client.username_pw_set(MQTT_USERNAME, MQTT_PASSWORD)
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
            print(f"[{self.plc_id}] MQTT连接成功，QoS={MQTT_QOS}")
            self.client.subscribe(CONTROL_TOPIC, qos=MQTT_QOS)
            print(f"[{self.plc_id}] 已订阅控制指令主题: {CONTROL_TOPIC} (QoS {MQTT_QOS})")
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
                command_id = payload.get("command_id", f"cmd_{int(time.time() * 1000)}")

                self.commands_received += 1
                print(f"[{self.plc_id}] 收到指令 ({msg.qos}): {target_type}/{target_id} {action}={value}, cmd_id={command_id}")

                results = []
                device = self.devices.get(target_id)
                if device:
                    result = device.execute_command(action, value)
                    results.append(result)
                else:
                    for dev in self.devices.values():
                        if dev.type == target_type:
                            result = dev.execute_command(action, value)
                            results.append(result)

                if results:
                    self._send_feedback(command_id, target_id, results)

        except Exception as e:
            print(f"[{self.plc_id}] 处理命令异常: {e}")
            self.communication_errors += 1

    def _send_feedback(self, command_id, target_id, results):
        feedback_topic = FEEDBACK_TOPIC.format(plc_id=self.plc_id)
        feedback = {
            "command_id": command_id,
            "plc_id": self.plc_id,
            "target_id": target_id,
            "timestamp": datetime.now().isoformat(),
            "results": results,
            "all_success": all(r["success"] for r in results),
        }

        try:
            payload = json.dumps(feedback, ensure_ascii=False)
            self.client.publish(feedback_topic, payload, qos=STATUS_TOPIC_QOS)
            print(f"[{self.plc_id}] 已发送执行反馈 -> {feedback_topic}")
        except Exception as e:
            print(f"[{self.plc_id}] 发送反馈失败: {e}")

    def update_devices(self):
        self.last_scan = datetime.now()
        for device in self.devices.values():
            device.update()

    def report_status(self):
        if not self.connected:
            if not self.connect_mqtt():
                return

        topic = STATUS_TOPIC.format(plc_id=self.plc_id)

        for device in self.devices.values():
            status = device.get_status()

            try:
                payload = json.dumps(status, ensure_ascii=False)
                self.client.publish(topic, payload, qos=STATUS_TOPIC_QOS)
            except Exception as e:
                print(f"[{self.plc_id}] 上报状态失败: {e}")
                self.communication_errors += 1

    def get_summary(self):
        running = sum(1 for d in self.devices.values() if d.status == "running")
        fault = sum(1 for d in self.devices.values() if d.status == "fault")
        stopped = sum(1 for d in self.devices.values() if d.status == "stopped")
        total_commands = sum(d.command_count for d in self.devices.values())
        total_success = sum(d.success_count for d in self.devices.values())
        total_failed = sum(d.failed_count for d in self.devices.values())

        return {
            "plc_id": self.plc_id,
            "name": self.name,
            "connected": self.connected,
            "total_devices": len(self.devices),
            "running_devices": running,
            "fault_devices": fault,
            "stopped_devices": stopped,
            "commands_received": self.commands_received,
            "commands_executed": total_commands,
            "commands_success": total_success,
            "commands_failed": total_failed,
            "communication_errors": self.communication_errors,
            "last_scan": self.last_scan.isoformat() if self.last_scan else None,
        }

    def disconnect(self):
        if self.client:
            self.client.loop_stop()
            self.client.disconnect()
        self.connected = False


def print_config():
    print("=" * 70)
    print("PLC 模拟器启动 (增强版)")
    print("=" * 70)
    print(f"MQTT Broker: {MQTT_BROKER}:{MQTT_PORT}")
    print(f"MQTT QoS: {MQTT_QOS}")
    print(f"状态上报QoS: {STATUS_TOPIC_QOS}")
    print(f"状态上报间隔: {REPORT_INTERVAL}秒")
    print(f"控制指令主题: {CONTROL_TOPIC}")
    print(f"设备状态主题: {STATUS_TOPIC}")
    print(f"执行反馈主题: {FEEDBACK_TOPIC}")
    print(f"故障模拟: {'启用' if FAULT_SIMULATION_ENABLED else '禁用'}")
    print(f"设备响应时间: {RESPONSE_TIME}秒")
    print("=" * 70)


def run_simulation():
    print_config()

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
    print("=" * 70)

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
                    print(f"\n[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] 运行状态:")
                    for plc in plcs:
                        summary = plc.get_summary()
                        success_rate = (summary['commands_success'] / max(1, summary['commands_executed'])) * 100
                        print(f"  {summary['plc_id']}: "
                              f"运行{summary['running_devices']}/{summary['total_devices']} "
                              f"故障{summary['fault_devices']} "
                              f"停止{summary['stopped_devices']} | "
                              f"指令: 收到{summary['commands_received']} "
                              f"执行{summary['commands_executed']} "
                              f"成功率{success_rate:.1f}% | "
                              f"通信错误{summary['communication_errors']}")

            time.sleep(0.1)

    except KeyboardInterrupt:
        print("\n\n收到停止信号，正在关闭...")
        for plc in plcs:
            plc.disconnect()

        print("\n" + "=" * 70)
        print("PLC模拟器停止，最终统计:")
        for plc in plcs:
            summary = plc.get_summary()
            success_rate = (summary['commands_success'] / max(1, summary['commands_executed'])) * 100
            print(f"  {summary['plc_id']}: "
                  f"总指令{summary['commands_executed']}, "
                  f"成功{summary['commands_success']}, "
                  f"失败{summary['commands_failed']}, "
                  f"成功率{success_rate:.1f}%")
        print("=" * 70)
        print("PLC模拟器已停止")


if __name__ == "__main__":
    import sys

    if len(sys.argv) > 1 and sys.argv[1] == "config":
        print_config()
    else:
        run_simulation()
