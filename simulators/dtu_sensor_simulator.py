#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
4G DTU 传感器模拟器
模拟污水处理厂生化池内的75台传感器每2分钟通过4G DTU上报数据
- 30台溶解氧(DO)传感器
- 20台氨氮(NH3-N)传感器
- 15台硝氮(NO3-N)传感器
- 10台磷酸盐(PO4-P)传感器
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
MQTT_TOPIC = "sensor/{sensor_id}/data"
REPORT_INTERVAL = 120

SENSOR_CONFIGS = {
    "DO": {
        "count": 30,
        "unit": "mg/L",
        "base_values": {
            "anaerobic": 0.2,
            "anoxic": 0.5,
            "aerobic1": 2.5,
            "aerobic2": 2.0,
            "aerobic3": 1.5,
            "effluent": 2.0,
        },
        "variation": 0.3,
    },
    "NH3": {
        "count": 20,
        "unit": "mg/L",
        "base_values": {
            "anaerobic": 35.0,
            "anoxic": 25.0,
            "aerobic1": 15.0,
            "aerobic2": 5.0,
            "aerobic3": 2.0,
            "effluent": 1.5,
        },
        "variation": 2.0,
    },
    "NO3": {
        "count": 15,
        "unit": "mg/L",
        "base_values": {
            "anaerobic": 2.0,
            "anoxic": 8.0,
            "aerobic1": 12.0,
            "aerobic2": 10.0,
            "aerobic3": 8.0,
            "effluent": 10.0,
        },
        "variation": 1.5,
    },
    "PO4": {
        "count": 10,
        "unit": "mg/L",
        "base_values": {
            "anaerobic": 6.0,
            "anoxic": 5.0,
            "aerobic1": 3.0,
            "aerobic2": 1.0,
            "aerobic3": 0.5,
            "effluent": 0.3,
        },
        "variation": 0.3,
    },
    "COD": {
        "count": 5,
        "unit": "mg/L",
        "base_values": {
            "influent": 350.0,
            "anaerobic": 300.0,
            "anoxic": 250.0,
            "aerobic1": 150.0,
            "effluent": 40.0,
        },
        "variation": 30.0,
    },
    "FLOW": {
        "count": 2,
        "unit": "m3/d",
        "base_values": {
            "influent": 300000.0,
            "effluent": 295000.0,
        },
        "variation": 15000.0,
    },
}

LOCATIONS = ["anaerobic", "anoxic", "aerobic1", "aerobic2", "aerobic3", "effluent"]


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

    def generate_value(self):
        if self.offline:
            return None

        self.drift += random.uniform(-0.01, 0.01)
        self.drift = max(-0.5, min(0.5, self.drift))

        time_factor = math.sin(time.time() / 3600 * 2 * math.pi) * 0.1
        random_factor = random.uniform(-1, 1) * self.variation * 0.3

        self.current_value = self.base_value * (1 + self.drift + time_factor) + random_factor
        self.current_value = max(0, self.current_value)

        if random.random() < 0.001:
            self.current_value = self.base_value * 2
            self.status = "warning"
        elif random.random() < 0.0005:
            self.current_value = self.base_value * 0.1
            self.status = "error"
        else:
            self.status = "normal"

        if random.random() < 0.0001:
            self.offline = True
            threading.Timer(60, self.reconnect).start()

        self.last_report = datetime.now()
        return self.current_value

    def reconnect(self):
        self.offline = False
        self.status = "normal"

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
        }


class DTUDevice:
    def __init__(self, dtu_id, sensors):
        self.dtu_id = dtu_id
        self.sensors = sensors
        self.client = None
        self.connected = False
        self.signal_strength = -70

    def connect_mqtt(self):
        self.client = mqtt.Client(client_id=self.dtu_id, clean_session=True)
        self.client.username_pw_set("admin", "admin123")
        self.client.on_connect = self.on_connect
        self.client.on_disconnect = self.on_disconnect

        try:
            self.client.connect(MQTT_BROKER, MQTT_PORT, 60)
            self.client.loop_start()
            return True
        except Exception as e:
            print(f"[{self.dtu_id}] MQTT连接失败: {e}")
            return False

    def on_connect(self, client, userdata, flags, rc):
        if rc == 0:
            self.connected = True
            print(f"[{self.dtu_id}] MQTT连接成功")
        else:
            print(f"[{self.dtu_id}] MQTT连接失败，错误码: {rc}")

    def on_disconnect(self, client, userdata, rc):
        self.connected = False
        print(f"[{self.dtu_id}] MQTT断开连接，错误码: {rc}")

    def report_data(self):
        if not self.connected:
            if not self.connect_mqtt():
                return

        self.signal_strength = random.randint(-90, -50)

        for sensor in self.sensors:
            data = sensor.get_data()
            if data is None:
                continue

            data["dtu_id"] = self.dtu_id
            data["signal_strength"] = self.signal_strength

            topic = MQTT_TOPIC.format(sensor_id=sensor.sensor_id)

            try:
                payload = json.dumps(data, ensure_ascii=False)
                result = self.client.publish(topic, payload, qos=1)
                if result.rc == mqtt.MQTT_ERR_SUCCESS:
                    print(f"[{self.dtu_id}] 上报 {sensor.sensor_id}: {data['value']} {data['unit']}")
                else:
                    print(f"[{self.dtu_id}] 上报失败 {sensor.sensor_id}")
            except Exception as e:
                print(f"[{self.dtu_id}] 上报异常: {e}")

            time.sleep(0.05)

    def disconnect(self):
        if self.client:
            self.client.loop_stop()
            self.client.disconnect()
        self.connected = False


def create_sensors():
    sensors = []

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

    return sensors


def create_dtu_devices(sensors):
    dtu_devices = []
    sensors_per_dtu = 10

    for i, start in enumerate(range(0, len(sensors), sensors_per_dtu)):
        dtu_sensors = sensors[start:start + sensors_per_dtu]
        dtu = DTUDevice(f"DTU-{str(i+1).zfill(3)}", dtu_sensors)
        dtu_devices.append(dtu)

    return dtu_devices


def run_simulation():
    print("=" * 60)
    print("4G DTU 传感器模拟器启动")
    print(f"MQTT Broker: {MQTT_BROKER}:{MQTT_PORT}")
    print(f"上报间隔: {REPORT_INTERVAL}秒")
    print("=" * 60)

    sensors = create_sensors()
    dtu_devices = create_dtu_devices(sensors)

    print(f"\n创建了 {len(sensors)} 台传感器，分配到 {len(dtu_devices)} 个DTU设备")

    sensor_counts = {}
    for sensor in sensors:
        sensor_counts[sensor.sensor_type] = sensor_counts.get(sensor.sensor_type, 0) + 1

    for st, count in sensor_counts.items():
        print(f"  {st}: {count}台")

    print("\n连接MQTT...")
    for dtu in dtu_devices:
        dtu.connect_mqtt()
        time.sleep(0.1)

    print("\n开始数据上报...")
    print("=" * 60)

    try:
        while True:
            start_time = time.time()

            threads = []
            for dtu in dtu_devices:
                t = threading.Thread(target=dtu.report_data)
                threads.append(t)
                t.start()

            for t in threads:
                t.join()

            elapsed = time.time() - start_time
            sleep_time = max(0, REPORT_INTERVAL - elapsed)

            print(f"\n本次上报完成，耗时: {elapsed:.1f}秒，休眠: {sleep_time:.1f}秒")
            print("-" * 60)

            time.sleep(sleep_time)

    except KeyboardInterrupt:
        print("\n\n收到停止信号，正在关闭...")
        for dtu in dtu_devices:
            dtu.disconnect()
        print("模拟器已停止")


def run_single_sensor_test():
    print("单传感器测试模式")
    print("=" * 60)

    sensor = SensorSimulator(
        sensor_type="DO",
        sensor_id="DO-001",
        location="aerobic1",
        base_value=2.0,
        unit="mg/L",
        variation=0.3,
    )

    dtu = DTUDevice("DTU-TEST", [sensor])
    dtu.connect_mqtt()

    try:
        for i in range(10):
            dtu.report_data()
            time.sleep(2)
    except KeyboardInterrupt:
        pass
    finally:
        dtu.disconnect()


if __name__ == "__main__":
    import sys

    if len(sys.argv) > 1 and sys.argv[1] == "test":
        run_single_sensor_test()
    else:
        run_simulation()
