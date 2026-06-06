#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
4G DTU 传感器模拟器 (增强版)
支持环境变量配置、水质波动调节、故障模拟
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
MQTT_TOPIC = os.environ.get('MQTT_TOPIC', 'sensor/{sensor_id}/data')
MQTT_QOS = get_env_int('MQTT_QOS', 1)
REPORT_INTERVAL = get_env_int('REPORT_INTERVAL', 120)

WATER_QUALITY_FLUCTUATION = get_env_float('WATER_QUALITY_FLUCTUATION', 1.0)
FAULT_SIMULATION_ENABLED = get_env_bool('FAULT_SIMULATION_ENABLED', True)
DRIFT_SIMULATION_ENABLED = get_env_bool('DRIFT_SIMULATION_ENABLED', True)

SENSOR_CONFIGS = {
    "DO": {
        "count": get_env_int('DO_SENSOR_COUNT', 28),
        "unit": "mg/L",
        "base_values": {
            "anaerobic": get_env_float('DO_BASE_ANAEROBIC', 0.2),
            "anoxic": get_env_float('DO_BASE_ANOXIC', 0.5),
            "aerobic1": get_env_float('DO_BASE_AEROBIC1', 2.5),
            "aerobic2": get_env_float('DO_BASE_AEROBIC2', 2.0),
            "aerobic3": get_env_float('DO_BASE_AEROBIC3', 1.5),
            "effluent": get_env_float('DO_BASE_EFFLUENT', 2.0),
        },
        "variation": get_env_float('DO_VARIATION', 0.3) * WATER_QUALITY_FLUCTUATION,
    },
    "NH3": {
        "count": get_env_int('NH3_SENSOR_COUNT', 18),
        "unit": "mg/L",
        "base_values": {
            "anaerobic": get_env_float('NH3_BASE_ANAEROBIC', 35.0),
            "anoxic": get_env_float('NH3_BASE_ANOXIC', 25.0),
            "aerobic1": get_env_float('NH3_BASE_AEROBIC1', 15.0),
            "aerobic2": get_env_float('NH3_BASE_AEROBIC2', 5.0),
            "aerobic3": get_env_float('NH3_BASE_AEROBIC3', 2.0),
            "effluent": get_env_float('NH3_BASE_EFFLUENT', 1.5),
        },
        "variation": get_env_float('NH3_VARIATION', 2.0) * WATER_QUALITY_FLUCTUATION,
    },
    "NO3": {
        "count": get_env_int('NO3_SENSOR_COUNT', 12),
        "unit": "mg/L",
        "base_values": {
            "anaerobic": get_env_float('NO3_BASE_ANAEROBIC', 2.0),
            "anoxic": get_env_float('NO3_BASE_ANOXIC', 8.0),
            "aerobic1": get_env_float('NO3_BASE_AEROBIC1', 12.0),
            "aerobic2": get_env_float('NO3_BASE_AEROBIC2', 10.0),
            "aerobic3": get_env_float('NO3_BASE_AEROBIC3', 8.0),
            "effluent": get_env_float('NO3_BASE_EFFLUENT', 10.0),
        },
        "variation": get_env_float('NO3_VARIATION', 1.5) * WATER_QUALITY_FLUCTUATION,
    },
    "PO4": {
        "count": get_env_int('PO4_SENSOR_COUNT', 10),
        "unit": "mg/L",
        "base_values": {
            "anaerobic": get_env_float('PO4_BASE_ANAEROBIC', 6.0),
            "anoxic": get_env_float('PO4_BASE_ANOXIC', 5.0),
            "aerobic1": get_env_float('PO4_BASE_AEROBIC1', 3.0),
            "aerobic2": get_env_float('PO4_BASE_AEROBIC2', 1.0),
            "aerobic3": get_env_float('PO4_BASE_AEROBIC3', 0.5),
            "effluent": get_env_float('PO4_BASE_EFFLUENT', 0.3),
        },
        "variation": get_env_float('PO4_VARIATION', 0.3) * WATER_QUALITY_FLUCTUATION,
    },
    "COD": {
        "count": get_env_int('COD_SENSOR_COUNT', 5),
        "unit": "mg/L",
        "base_values": {
            "influent": get_env_float('COD_BASE_INFLUENT', 350.0),
            "anaerobic": get_env_float('COD_BASE_ANAEROBIC', 300.0),
            "anoxic": get_env_float('COD_BASE_ANOXIC', 250.0),
            "aerobic1": get_env_float('COD_BASE_AEROBIC1', 150.0),
            "effluent": get_env_float('COD_BASE_EFFLUENT', 40.0),
        },
        "variation": get_env_float('COD_VARIATION', 30.0) * WATER_QUALITY_FLUCTUATION,
    },
    "FLOW": {
        "count": get_env_int('FLOW_SENSOR_COUNT', 2),
        "unit": "m3/d",
        "base_values": {
            "influent": get_env_float('FLOW_BASE_INFLUENT', 300000.0),
            "effluent": get_env_float('FLOW_BASE_EFFLUENT', 295000.0),
        },
        "variation": get_env_float('FLOW_VARIATION', 15000.0) * WATER_QUALITY_FLUCTUATION,
    },
}

LOCATIONS = ["anaerobic", "anoxic", "aerobic1", "aerobic2", "aerobic3", "effluent", "influent"]


class SensorSimulator:
    def __init__(self, sensor_type, sensor_id, location, base_value, unit, variation):
        self.sensor_type = sensor_type
        self.sensor_id = sensor_id
        self.location = location
        self.base_value = base_value
        self.unit = unit
        self.variation = variation
        self.current_value = base_value
        self.status = "normal"
        self.offline = False
        self.drift = 0
        self.last_report = None
        self.report_count = 0
        self.error_count = 0

    def generate_value(self):
        if self.offline:
            return None

        if DRIFT_SIMULATION_ENABLED:
            self.drift += random.uniform(-0.005, 0.005)
            self.drift = max(-0.3, min(0.3, self.drift))

        time_factor = math.sin(time.time() / 3600 * 2 * math.pi) * 0.08 * WATER_QUALITY_FLUCTUATION
        random_factor = random.uniform(-1, 1) * self.variation * 0.25

        self.current_value = self.base_value * (1 + self.drift + time_factor) + random_factor
        self.current_value = max(0, self.current_value)

        if FAULT_SIMULATION_ENABLED:
            if random.random() < 0.0005:
                self.current_value = self.base_value * 2.5
                self.status = "warning"
                self.error_count += 1
            elif random.random() < 0.0002:
                self.current_value = self.base_value * 0.05
                self.status = "error"
                self.error_count += 1
            else:
                self.status = "normal"

            if random.random() < 0.00005:
                self.offline = True
                offline_duration = random.randint(30, 120)
                threading.Timer(offline_duration, self.reconnect).start()
                print(f"[{self.sensor_id}] 模拟离线，持续{offline_duration}秒")

        self.last_report = datetime.now()
        self.report_count += 1
        return self.current_value

    def reconnect(self):
        self.offline = False
        self.status = "normal"
        print(f"[{self.sensor_id}] 恢复连接")

    def get_data(self):
        value = self.generate_value()
        if value is None:
            return None

        return {
            "sensor_id": self.sensor_id,
            "type": self.sensor_type,
            "value": round(value, 3),
            "unit": self.unit,
            "location": self.location,
            "timestamp": self.last_report.isoformat(),
            "status": self.status,
            "report_count": self.report_count,
            "error_count": self.error_count,
        }


class DTUDevice:
    def __init__(self, dtu_id, sensors):
        self.dtu_id = dtu_id
        self.sensors = sensors
        self.client = None
        self.connected = False
        self.signal_strength = -70
        self.reconnect_attempts = 0
        self.total_reports = 0
        self.failed_reports = 0

    def connect_mqtt(self):
        client_id = f"{self.dtu_id}_{int(time.time())}"
        self.client = mqtt.Client(client_id=client_id, clean_session=True)
        self.client.username_pw_set(MQTT_USERNAME, MQTT_PASSWORD)
        self.client.on_connect = self.on_connect
        self.client.on_disconnect = self.on_disconnect
        self.client.on_publish = self.on_publish

        try:
            self.client.connect(MQTT_BROKER, MQTT_PORT, 60)
            self.client.loop_start()
            return True
        except Exception as e:
            self.reconnect_attempts += 1
            print(f"[{self.dtu_id}] MQTT连接失败 (尝试{self.reconnect_attempts}): {e}")
            return False

    def on_connect(self, client, userdata, flags, rc):
        if rc == 0:
            self.connected = True
            self.reconnect_attempts = 0
            print(f"[{self.dtu_id}] MQTT连接成功")
        else:
            print(f"[{self.dtu_id}] MQTT连接失败，错误码: {rc}")

    def on_disconnect(self, client, userdata, rc):
        self.connected = False
        print(f"[{self.dtu_id}] MQTT断开连接，错误码: {rc}")

    def on_publish(self, client, userdata, mid):
        pass

    def report_data(self):
        if not self.connected:
            if not self.connect_mqtt():
                self.failed_reports += len(self.sensors)
                return

        self.signal_strength = random.randint(-95, -45)

        for sensor in self.sensors:
            data = sensor.get_data()
            if data is None:
                continue

            data["dtu_id"] = self.dtu_id
            data["signal_strength"] = self.signal_strength

            topic = MQTT_TOPIC.format(sensor_id=sensor.sensor_id)

            try:
                payload = json.dumps(data, ensure_ascii=False)
                result = self.client.publish(topic, payload, qos=MQTT_QOS)
                if result.rc == mqtt.MQTT_ERR_SUCCESS:
                    self.total_reports += 1
                    if self.total_reports % 50 == 0:
                        print(f"[{self.dtu_id}] 上报 {sensor.sensor_id}: {data['value']} {data['unit']} "
                              f"(总计:{self.total_reports}, 失败:{self.failed_reports})")
                else:
                    self.failed_reports += 1
                    print(f"[{self.dtu_id}] 上报失败 {sensor.sensor_id}, rc={result.rc}")
            except Exception as e:
                self.failed_reports += 1
                print(f"[{self.dtu_id}] 上报异常: {e}")

            time.sleep(0.03)

    def disconnect(self):
        if self.client:
            self.client.loop_stop()
            self.client.disconnect()
        self.connected = False

    def get_stats(self):
        return {
            "dtu_id": self.dtu_id,
            "connected": self.connected,
            "sensor_count": len(self.sensors),
            "total_reports": self.total_reports,
            "failed_reports": self.failed_reports,
            "signal_strength": self.signal_strength,
            "reconnect_attempts": self.reconnect_attempts,
        }


def create_sensors():
    sensors = []
    total_count = 0

    for sensor_type, config in SENSOR_CONFIGS.items():
        for i in range(config["count"]):
            location_index = min(i // max(1, config["count"] // len(LOCATIONS)), len(LOCATIONS) - 1)
            location = LOCATIONS[location_index]

            if location in config["base_values"]:
                base_value = config["base_values"][location]
            else:
                base_value = list(config["base_values"].values())[0]

            sensor = SensorSimulator(
                sensor_type=sensor_type,
                sensor_id=f"{sensor_type}-{str(i+1).zfill(3)}",
                location=location,
                base_value=base_value,
                unit=config["unit"],
                variation=config["variation"],
            )
            sensors.append(sensor)
            total_count += 1

    print(f"\n配置传感器总数: {total_count}台")
    return sensors


def create_dtu_devices(sensors):
    dtu_devices = []
    sensors_per_dtu = get_env_int('SENSORS_PER_DTU', 10)

    for i, start in enumerate(range(0, len(sensors), sensors_per_dtu)):
        dtu_sensors = sensors[start:start + sensors_per_dtu]
        dtu = DTUDevice(f"DTU-{str(i+1).zfill(3)}", dtu_sensors)
        dtu_devices.append(dtu)

    return dtu_devices


def print_config():
    print("=" * 70)
    print("4G DTU 传感器模拟器启动 (增强版)")
    print("=" * 70)
    print(f"MQTT Broker: {MQTT_BROKER}:{MQTT_PORT}")
    print(f"MQTT QoS: {MQTT_QOS}")
    print(f"上报间隔: {REPORT_INTERVAL}秒")
    print(f"水质波动系数: {WATER_QUALITY_FLUCTUATION}x")
    print(f"故障模拟: {'启用' if FAULT_SIMULATION_ENABLED else '禁用'}")
    print(f"漂移模拟: {'启用' if DRIFT_SIMULATION_ENABLED else '禁用'}")
    print("=" * 70)

    sensor_counts = {}
    for sensor_type, config in SENSOR_CONFIGS.items():
        sensor_counts[sensor_type] = config["count"]
        print(f"  {sensor_type}: {config['count']}台 (波动±{config['variation']:.2f})")
    print("=" * 70)


def run_simulation():
    print_config()

    sensors = create_sensors()
    dtu_devices = create_dtu_devices(sensors)

    print(f"\n创建了 {len(sensors)} 台传感器，分配到 {len(dtu_devices)} 个DTU设备")

    print("\n连接MQTT...")
    for dtu in dtu_devices:
        dtu.connect_mqtt()
        time.sleep(0.1)

    print("\n开始数据上报...")
    print("=" * 70)

    last_stats = time.time()
    cycle_count = 0

    try:
        while True:
            start_time = time.time()
            cycle_count += 1

            threads = []
            for dtu in dtu_devices:
                t = threading.Thread(target=dtu.report_data)
                threads.append(t)
                t.start()

            for t in threads:
                t.join()

            elapsed = time.time() - start_time
            sleep_time = max(0, REPORT_INTERVAL - elapsed)

            if time.time() - last_stats >= 300:
                last_stats = time.time()
                print(f"\n[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] 运行统计 (周期{cycle_count}):")
                total_reports = sum(d.total_reports for d in dtu_devices)
                total_failed = sum(d.failed_reports for d in dtu_devices)
                print(f"  总上报: {total_reports}, 失败: {total_failed}")
                for dtu in dtu_devices[:3]:
                    stats = dtu.get_stats()
                    print(f"  {stats['dtu_id']}: 信号{stats['signal_strength']}dBm, "
                          f"上报{stats['total_reports']}, 失败{stats['failed_reports']}")
                print("-" * 70)

            if cycle_count % 10 == 0:
                print(f"\n[{datetime.now().strftime('%H:%M:%S')}] 周期{cycle_count}完成，"
                      f"耗时: {elapsed:.1f}秒，休眠: {sleep_time:.1f}秒")

            time.sleep(sleep_time)

    except KeyboardInterrupt:
        print("\n\n收到停止信号，正在关闭...")
        for dtu in dtu_devices:
            dtu.disconnect()

        print("\n" + "=" * 70)
        print("模拟器停止，最终统计:")
        total_reports = sum(d.total_reports for d in dtu_devices)
        total_failed = sum(d.failed_reports for d in dtu_devices)
        print(f"  总上报次数: {total_reports}")
        print(f"  失败次数: {total_failed}")
        print(f"  成功率: {(1 - total_failed / max(1, total_reports)) * 100:.2f}%")
        print("=" * 70)
        print("模拟器已停止")


if __name__ == "__main__":
    import sys

    if len(sys.argv) > 1 and sys.argv[1] == "config":
        print_config()
    else:
        run_simulation()
