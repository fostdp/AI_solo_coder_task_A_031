#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
4G DTU 传感器模拟器
模拟城市污水处理厂生化池内的传感器数据上报
每2分钟通过MQTT上报一次数据
"""

import json
import time
import random
import math
from datetime import datetime
import paho.mqtt.client as mqtt

MQTT_BROKER = "localhost"
MQTT_PORT = 1883
MQTT_TOPIC_PREFIX = "sewage/sensor"
MQTT_USERNAME = ""
MQTT_PASSWORD = ""

REPORT_INTERVAL = 120

SENSOR_CONFIGS = []

for i in range(1, 31):
    section = ((i - 1) // 10) + 1
    idx = ((i - 1) % 10) + 1
    SENSOR_CONFIGS.append({
        "id": f"DO-{chr(64 + section)}-{idx}",
        "type": "DO",
        "stage": "aerobic",
        "section": section,
        "unit": "mg/L",
        "target_min": 1.5,
        "target_max": 2.5,
        "base_value": 2.0,
        "variation": 0.5
    })

for i in range(1, 21):
    section = ((i - 1) // 7) + 1
    idx = ((i - 1) % 7) + 1
    SENSOR_CONFIGS.append({
        "id": f"NH3-{chr(64 + section)}-{idx}",
        "type": "NH3",
        "stage": "aerobic",
        "section": section,
        "unit": "mg/L",
        "target_min": 1.0,
        "target_max": 2.0,
        "base_value": 1.5,
        "variation": 0.8
    })

for i in range(1, 16):
    section = ((i - 1) // 5) + 1
    idx = ((i - 1) % 5) + 1
    SENSOR_CONFIGS.append({
        "id": f"NO3-{chr(64 + section)}-{idx}",
        "type": "NO3",
        "stage": "anoxic",
        "section": section,
        "unit": "mg/L",
        "target_min": 0.5,
        "target_max": 3.0,
        "base_value": 2.0,
        "variation": 1.0
    })

for i in range(1, 11):
    SENSOR_CONFIGS.append({
        "id": f"PO4-{i}",
        "type": "PO4",
        "stage": "anaerobic",
        "section": 1,
        "unit": "mg/L",
        "target_min": 0.3,
        "target_max": 1.0,
        "base_value": 0.6,
        "variation": 0.3
    })

SENSOR_CONFIGS.append({
    "id": "COD-IN",
    "type": "COD",
    "stage": "primary_settling",
    "section": 1,
    "unit": "mg/L",
    "target_min": 200,
    "target_max": 400,
    "base_value": 300,
    "variation": 80
})

SENSOR_CONFIGS.append({
    "id": "TN-EFF",
    "type": "TN",
    "stage": "effluent",
    "section": 1,
    "unit": "mg/L",
    "target_min": 5,
    "target_max": 15,
    "base_value": 12,
    "variation": 3
})

SENSOR_CONFIGS.append({
    "id": "NH3-EFF",
    "type": "NH3",
    "stage": "effluent",
    "section": 1,
    "unit": "mg/L",
    "target_min": 0.5,
    "target_max": 1.5,
    "base_value": 1.0,
    "variation": 0.5
})

sensor_values = {}
anomaly_sensors = set()

def generate_sensor_value(config, timestamp):
    sensor_id = config["id"]
    
    if sensor_id not in sensor_values:
        sensor_values[sensor_id] = config["base_value"]
    
    time_hour = timestamp.hour
    daily_factor = 1.0 + 0.15 * math.sin(2 * math.pi * (time_hour - 6) / 24)
    
    random_walk = random.gauss(0, config["variation"] * 0.1)
    new_value = sensor_values[sensor_id] + random_walk
    
    target = (config["target_min"] + config["target_max"]) / 2
    range_val = config["target_max"] - config["target_min"]
    
    mean_reversion = (target - new_value) * 0.05
    new_value += mean_reversion
    
    new_value *= daily_factor
    
    if sensor_id in anomaly_sensors:
        anomaly_factor = random.choice([0.3, 1.8])
        new_value *= anomaly_factor
        print(f"[异常] {sensor_id} 模拟异常数据: {new_value:.2f} {config['unit']}")
        anomaly_sensors.discard(sensor_id)
    
    min_val = config["target_min"] * 0.2
    max_val = config["target_max"] * 2.0
    new_value = max(min_val, min(max_val, new_value))
    
    sensor_values[sensor_id] = new_value
    return new_value

def on_connect(client, userdata, flags, rc):
    if rc == 0:
        print(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] MQTT 连接成功")
    else:
        print(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] MQTT 连接失败，错误码: {rc}")

def on_disconnect(client, userdata, rc):
    print(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] MQTT 断开连接，错误码: {rc}")

def publish_sensor_data(client, config, timestamp):
    value = generate_sensor_value(config, timestamp)
    
    data = {
        "id": config["id"],
        "type": config["type"],
        "stage": config["stage"],
        "section": config["section"],
        "value": round(value, 3),
        "unit": config["unit"],
        "timestamp": timestamp.isoformat(),
        "status": "online",
        "alarm_level": 0
    }
    
    topic = f"{MQTT_TOPIC_PREFIX}/{config['id']}"
    payload = json.dumps(data, ensure_ascii=False)
    
    result = client.publish(topic, payload, qos=1)
    if result.rc == mqtt.MQTT_ERR_SUCCESS:
        return True
    else:
        print(f"[错误] 发布失败 {config['id']}: {result.rc}")
        return False

def simulate_dtu():
    client = mqtt.Client(client_id="DTU_SIMULATOR_" + str(random.randint(1000, 9999)))
    client.on_connect = on_connect
    client.on_disconnect = on_disconnect
    
    if MQTT_USERNAME:
        client.username_pw_set(MQTT_USERNAME, MQTT_PASSWORD)
    
    try:
        client.connect(MQTT_BROKER, MQTT_PORT, keepalive=60)
        client.loop_start()
    except Exception as e:
        print(f"[错误] 无法连接到MQTT服务器: {e}")
        print("请确保MQTT服务器已启动，或检查连接配置")
        return
    
    print("=" * 70)
    print("4G DTU 传感器模拟器启动")
    print("=" * 70)
    print(f"MQTT Broker: {MQTT_BROKER}:{MQTT_PORT}")
    print(f"上报间隔: {REPORT_INTERVAL}秒")
    print(f"传感器数量: {len(SENSOR_CONFIGS)}")
    print("=" * 70)
    print("传感器列表:")
    for cfg in SENSOR_CONFIGS:
        print(f"  {cfg['id']:12s} - {cfg['type']:5s} - {cfg['stage']}")
    print("=" * 70)
    print("输入 'anomaly <sensor_id>' 可模拟传感器异常")
    print("输入 'quit' 退出程序")
    print("=" * 70)
    
    import threading
    import sys
    
    def handle_input():
        while True:
            try:
                line = sys.stdin.readline().strip()
                if not line:
                    continue
                if line.lower() == 'quit':
                    print("正在退出...")
                    client.loop_stop()
                    client.disconnect()
                    sys.exit(0)
                elif line.startswith('anomaly '):
                    sensor_id = line.split(' ', 1)[1].strip()
                    found = False
                    for cfg in SENSOR_CONFIGS:
                        if cfg['id'] == sensor_id:
                            anomaly_sensors.add(sensor_id)
                            print(f"已设置传感器 {sensor_id} 下次上报时模拟异常")
                            found = True
                            break
                    if not found:
                        print(f"未找到传感器 {sensor_id}")
                else:
                    print(f"未知命令: {line}")
            except KeyboardInterrupt:
                break
            except Exception as e:
                print(f"输入错误: {e}")
    
    input_thread = threading.Thread(target=handle_input, daemon=True)
    input_thread.start()
    
    report_count = 0
    try:
        while True:
            timestamp = datetime.now()
            success_count = 0
            
            print(f"\n[{timestamp.strftime('%Y-%m-%d %H:%M:%S')}] 开始第 {report_count + 1} 次上报...")
            
            for i, config in enumerate(SENSOR_CONFIGS):
                if publish_sensor_data(client, config, timestamp):
                    success_count += 1
                
                if (i + 1) % 15 == 0:
                    time.sleep(0.1)
            
            print(f"[{timestamp.strftime('%Y-%m-%d %H:%M:%S')}] 上报完成: {success_count}/{len(SENSOR_CONFIGS)} 成功")
            
            do_values = [sensor_values.get(c['id'], 0) for c in SENSOR_CONFIGS if c['type'] == 'DO']
            nh3_values = [sensor_values.get(c['id'], 0) for c in SENSOR_CONFIGS if c['type'] == 'NH3']
            
            if do_values:
                print(f"  DO 范围: {min(do_values):.2f} - {max(do_values):.2f} mg/L")
            if nh3_values:
                print(f"  NH3范围: {min(nh3_values):.2f} - {max(nh3_values):.2f} mg/L")
            
            if 'NH3-EFF' in sensor_values:
                print(f"  出水NH3: {sensor_values['NH3-EFF']:.2f} mg/L")
            if 'TN-EFF' in sensor_values:
                print(f"  出水TN : {sensor_values['TN-EFF']:.2f} mg/L")
            if 'COD-IN' in sensor_values:
                print(f"  进水COD: {sensor_values['COD-IN']:.1f} mg/L")
            
            report_count += 1
            
            time.sleep(REPORT_INTERVAL)
            
    except KeyboardInterrupt:
        print("\n\n收到中断信号，正在退出...")
    finally:
        client.loop_stop()
        client.disconnect()
        print("模拟器已停止")

if __name__ == "__main__":
    simulate_dtu()
