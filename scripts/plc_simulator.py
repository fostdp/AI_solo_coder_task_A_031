#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
PLC 模拟器
模拟污水处理厂的鼓风机、阀门和碳源投加泵的PLC控制
接收MQTT控制指令并模拟执行，上报执行状态
"""

import json
import time
import random
import threading
from datetime import datetime
import paho.mqtt.client as mqtt

MQTT_BROKER = "localhost"
MQTT_PORT = 1883
MQTT_CMD_TOPIC = "sewage/command/#"
MQTT_RESPONSE_TOPIC = "sewage/command/response"
MQTT_USERNAME = ""
MQTT_PASSWORD = ""

PLC_DEVICES = {
    "blower_1": {
        "name": "1号曝气风机",
        "type": "blower",
        "status": "running",
        "current_speed": 50.0,
        "target_speed": 50.0,
        "max_speed": 100.0,
        "min_speed": 20.0,
        "power": 0.0,
        "flow_rate": 0.0,
        "vibration": 2.5,
        "temperature": 45.0,
        "fault": False
    },
    "blower_2": {
        "name": "2号曝气风机",
        "type": "blower",
        "status": "running",
        "current_speed": 50.0,
        "target_speed": 50.0,
        "max_speed": 100.0,
        "min_speed": 20.0,
        "power": 0.0,
        "flow_rate": 0.0,
        "vibration": 2.3,
        "temperature": 43.0,
        "fault": False
    },
    "blower_3": {
        "name": "3号曝气风机",
        "type": "blower",
        "status": "running",
        "current_speed": 50.0,
        "target_speed": 50.0,
        "max_speed": 100.0,
        "min_speed": 20.0,
        "power": 0.0,
        "flow_rate": 0.0,
        "vibration": 2.7,
        "temperature": 47.0,
        "fault": False
    },
    "valve_aerobic_1": {
        "name": "好氧池A段曝气阀",
        "type": "valve",
        "status": "open",
        "current_opening": 50.0,
        "target_opening": 50.0,
        "max_opening": 100.0,
        "min_opening": 0.0,
        "flow_rate": 0.0,
        "pressure": 0.35,
        "fault": False
    },
    "valve_aerobic_2": {
        "name": "好氧池B段曝气阀",
        "type": "valve",
        "status": "open",
        "current_opening": 50.0,
        "target_opening": 50.0,
        "max_opening": 100.0,
        "min_opening": 0.0,
        "flow_rate": 0.0,
        "pressure": 0.34,
        "fault": False
    },
    "valve_aerobic_3": {
        "name": "好氧池C段曝气阀",
        "type": "valve",
        "status": "open",
        "current_opening": 50.0,
        "target_opening": 50.0,
        "max_opening": 100.0,
        "min_opening": 0.0,
        "flow_rate": 0.0,
        "pressure": 0.36,
        "fault": False
    },
    "valve_carbon": {
        "name": "碳源投加阀",
        "type": "valve",
        "status": "open",
        "current_opening": 50.0,
        "target_opening": 50.0,
        "max_opening": 100.0,
        "min_opening": 0.0,
        "flow_rate": 0.0,
        "pressure": 0.25,
        "fault": False
    },
    "carbon_pump": {
        "name": "碳源投加泵",
        "type": "pump",
        "status": "running",
        "current_dosage": 50.0,
        "target_dosage": 50.0,
        "max_dosage": 200.0,
        "min_dosage": 10.0,
        "flow_rate": 0.0,
        "pressure": 0.3,
        "speed": 1500.0,
        "fault": False
    }
}

command_history = []
faulty_devices = set()

def on_connect(client, userdata, flags, rc):
    if rc == 0:
        print(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] PLC 连接成功")
        client.subscribe(MQTT_CMD_TOPIC, qos=1)
        print(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] 已订阅控制指令主题: {MQTT_CMD_TOPIC}")
    else:
        print(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] PLC 连接失败，错误码: {rc}")

def on_disconnect(client, userdata, rc):
    print(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] PLC 断开连接，错误码: {rc}")

def on_message(client, userdata, msg):
    try:
        topic = msg.topic
        payload = json.loads(msg.payload.decode('utf-8'))
        
        device_id = topic.split('/')[-1]
        
        print(f"\n[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] 收到控制指令")
        print(f"  主题: {topic}")
        print(f"  设备: {device_id}")
        print(f"  指令: {json.dumps(payload, ensure_ascii=False, indent=2)}")
        
        process_command(client, device_id, payload)
        
    except Exception as e:
        print(f"[错误] 处理指令失败: {e}")
        print(f"  主题: {msg.topic}")
        print(f"  内容: {msg.payload}")

def process_command(client, device_id, payload):
    if device_id not in PLC_DEVICES:
        print(f"[警告] 未知设备: {device_id}")
        send_response(client, device_id, "error", f"未知设备: {device_id}", payload)
        return
    
    device = PLC_DEVICES[device_id]
    
    if device_id in faulty_devices:
        print(f"[故障] {device['name']} 处于故障状态，无法执行指令")
        send_response(client, device_id, "fault", f"{device['name']} 故障", payload)
        return
    
    cmd_type = payload.get("type", "")
    cmd_value = payload.get("value", 0)
    cmd_unit = payload.get("unit", "")
    cmd_source = payload.get("source", "")
    
    if device["type"] == "blower":
        target_speed = max(device["min_speed"], min(device["max_speed"], cmd_value))
        device["target_speed"] = target_speed
        device["status"] = "adjusting"
        print(f"  -> {device['name']}: 目标转速 {target_speed:.1f}%")
        
    elif device["type"] == "valve":
        target_opening = max(device["min_opening"], min(device["max_opening"], cmd_value))
        device["target_opening"] = target_opening
        device["status"] = "adjusting"
        print(f"  -> {device['name']}: 目标开度 {target_opening:.1f}%")
        
    elif device["type"] == "pump":
        target_dosage = max(device["min_dosage"], min(device["max_dosage"], cmd_value))
        device["target_dosage"] = target_dosage
        device["status"] = "adjusting"
        print(f"  -> {device['name']}: 目标投加量 {target_dosage:.1f} L/h")
    
    command_history.append({
        "timestamp": datetime.now(),
        "device_id": device_id,
        "command": payload,
        "status": "processing"
    })
    
    threading.Thread(target=execute_command, args=(client, device_id, payload), daemon=True).start()
    
    send_response(client, device_id, "accepted", f"{device['name']} 指令已接收", payload)

def execute_command(client, device_id, payload):
    device = PLC_DEVICES[device_id]
    
    time.sleep(random.uniform(0.5, 2.0))
    
    if device_id in faulty_devices:
        device["status"] = "fault"
        send_response(client, device_id, "fault", f"{device['name']} 执行时发生故障", payload)
        return
    
    device["status"] = "running"
    
    if device["type"] == "blower":
        adjust_speed(device)
        device["power"] = 120 * (device["current_speed"] / 50.0)
        device["flow_rate"] = 3000 * (device["current_speed"] / 100.0)
        device["vibration"] = 2.0 + device["current_speed"] * 0.02
        device["temperature"] = 35.0 + device["current_speed"] * 0.15
        
    elif device["type"] == "valve":
        adjust_valve(device)
        device["flow_rate"] = device["current_opening"] * 10.0
        
    elif device["type"] == "pump":
        adjust_pump(device)
        device["flow_rate"] = device["current_dosage"]
    
    if random.random() < 0.02:
        device["fault"] = True
        device["status"] = "fault"
        print(f"[故障] {device['name']} 模拟故障触发")
        send_response(client, device_id, "fault", f"{device['name']} 发生故障", payload)
        return
    
    send_response(client, device_id, "completed", f"{device['name']} 执行完成", payload)

def adjust_speed(device):
    step = 2.0
    diff = device["target_speed"] - device["current_speed"]
    
    if abs(diff) <= step:
        device["current_speed"] = device["target_speed"]
    elif diff > 0:
        device["current_speed"] += step
    else:
        device["current_speed"] -= step
    
    if device["current_speed"] != device["target_speed"]:
        threading.Timer(0.5, adjust_speed, args=(device,)).start()

def adjust_valve(device):
    step = 3.0
    diff = device["target_opening"] - device["current_opening"]
    
    if abs(diff) <= step:
        device["current_opening"] = device["target_opening"]
    elif diff > 0:
        device["current_opening"] += step
    else:
        device["current_opening"] -= step
    
    if device["current_opening"] != device["target_opening"]:
        threading.Timer(0.3, adjust_valve, args=(device,)).start()

def adjust_pump(device):
    step = 5.0
    diff = device["target_dosage"] - device["current_dosage"]
    
    if abs(diff) <= step:
        device["current_dosage"] = device["target_dosage"]
    elif diff > 0:
        device["current_dosage"] += step
    else:
        device["current_dosage"] -= step
    
    if device["current_dosage"] != device["target_dosage"]:
        threading.Timer(0.8, adjust_pump, args=(device,)).start()

def send_response(client, device_id, status, message, original_cmd):
    response = {
        "id": f"resp_{int(time.time() * 1000)}",
        "device_id": device_id,
        "device_name": PLC_DEVICES[device_id]["name"],
        "status": status,
        "message": message,
        "original_command": original_cmd,
        "device_state": get_device_state(device_id),
        "timestamp": datetime.now().isoformat()
    }
    
    payload = json.dumps(response, ensure_ascii=False)
    client.publish(MQTT_RESPONSE_TOPIC, payload, qos=1)
    
    status_icon = {"accepted": "✓", "completed": "✓✓", "error": "✗", "fault": "⚠"}
    print(f"  [{status_icon.get(status, '?')}] {message}")

def get_device_state(device_id):
    device = PLC_DEVICES[device_id]
    state = {
        "status": device["status"],
        "fault": device["fault"]
    }
    
    if device["type"] == "blower":
        state.update({
            "current_speed": round(device["current_speed"], 2),
            "target_speed": round(device["target_speed"], 2),
            "power": round(device["power"], 2),
            "flow_rate": round(device["flow_rate"], 2),
            "vibration": round(device["vibration"], 2),
            "temperature": round(device["temperature"], 2)
        })
    elif device["type"] == "valve":
        state.update({
            "current_opening": round(device["current_opening"], 2),
            "target_opening": round(device["target_opening"], 2),
            "flow_rate": round(device["flow_rate"], 2),
            "pressure": round(device["pressure"], 2)
        })
    elif device["type"] == "pump":
        state.update({
            "current_dosage": round(device["current_dosage"], 2),
            "target_dosage": round(device["target_dosage"], 2),
            "flow_rate": round(device["flow_rate"], 2),
            "pressure": round(device["pressure"], 2),
            "speed": round(device["speed"], 2)
        })
    
    return state

def monitor_loop():
    while True:
        time.sleep(5)
        
        timestamp = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
        print(f"\n[{timestamp}] PLC 状态监测")
        print("-" * 70)
        
        for device_id, device in PLC_DEVICES.items():
            fault_str = " ⚠故障" if device["fault"] or device_id in faulty_devices else ""
            status_str = f"[{device['status']}]"
            
            if device["type"] == "blower":
                print(f"  {device['name']:20s} {status_str:12s} 转速: {device['current_speed']:5.1f}%  功率: {device['power']:6.1f}kW  温度: {device['temperature']:5.1f}°C{fault_str}")
            elif device["type"] == "valve":
                print(f"  {device['name']:20s} {status_str:12s} 开度: {device['current_opening']:5.1f}%  流量: {device['flow_rate']:6.1f}L/min{fault_str}")
            elif device["type"] == "pump":
                print(f"  {device['name']:20s} {status_str:12s} 投量: {device['current_dosage']:5.1f}L/h 流量: {device['flow_rate']:6.1f}L/h{fault_str}")
        
        print("-" * 70)

def simulate_plc():
    client = mqtt.Client(client_id="PLC_SIMULATOR_" + str(random.randint(1000, 9999)))
    client.on_connect = on_connect
    client.on_disconnect = on_disconnect
    client.on_message = on_message
    
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
    print("PLC 模拟器启动")
    print("=" * 70)
    print(f"MQTT Broker: {MQTT_BROKER}:{MQTT_PORT}")
    print(f"监听主题: {MQTT_CMD_TOPIC}")
    print("=" * 70)
    print("设备列表:")
    for device_id, device in PLC_DEVICES.items():
        print(f"  {device_id:20s} - {device['name']} ({device['type']})")
    print("=" * 70)
    print("交互命令:")
    print("  status              - 显示设备状态")
    print("  fault <device_id>  - 模拟设备故障")
    print("  repair <device_id> - 修复设备故障")
    print("  history             - 显示命令历史")
    print("  quit                - 退出程序")
    print("=" * 70)
    
    import sys
    
    monitor_thread = threading.Thread(target=monitor_loop, daemon=True)
    monitor_thread.start()
    
    def handle_input():
        while True:
            try:
                line = sys.stdin.readline().strip()
                if not line:
                    continue
                
                parts = line.split()
                cmd = parts[0].lower()
                
                if cmd == 'quit':
                    print("正在退出...")
                    client.loop_stop()
                    client.disconnect()
                    sys.exit(0)
                
                elif cmd == 'status':
                    print("\n设备状态:")
                    for device_id, device in PLC_DEVICES.items():
                        state = get_device_state(device_id)
                        fault_str = " (故障)" if device["fault"] else ""
                        print(f"  {device_id}: {state['status']}{fault_str}")
                
                elif cmd == 'fault' and len(parts) > 1:
                    device_id = parts[1]
                    if device_id in PLC_DEVICES:
                        faulty_devices.add(device_id)
                        PLC_DEVICES[device_id]["fault"] = True
                        PLC_DEVICES[device_id]["status"] = "fault"
                        print(f"已设置 {PLC_DEVICES[device_id]['name']} 为故障状态")
                    else:
                        print(f"未找到设备: {device_id}")
                
                elif cmd == 'repair' and len(parts) > 1:
                    device_id = parts[1]
                    if device_id in PLC_DEVICES:
                        faulty_devices.discard(device_id)
                        PLC_DEVICES[device_id]["fault"] = False
                        PLC_DEVICES[device_id]["status"] = "running"
                        print(f"已修复 {PLC_DEVICES[device_id]['name']}")
                    else:
                        print(f"未找到设备: {device_id}")
                
                elif cmd == 'history':
                    print(f"\n命令历史 (共 {len(command_history)} 条):")
                    recent = command_history[-10:]
                    for i, cmd in enumerate(recent, 1):
                        print(f"  {i}. [{cmd['timestamp'].strftime('%H:%M:%S')}] {cmd['device_id']} - {cmd['command'].get('type', '')}: {cmd['command'].get('value', '')}")
                
                else:
                    print(f"未知命令: {line}")
                    
            except KeyboardInterrupt:
                break
            except Exception as e:
                print(f"输入错误: {e}")
    
    input_thread = threading.Thread(target=handle_input, daemon=True)
    input_thread.start()
    
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print("\n\n收到中断信号，正在退出...")
    finally:
        client.loop_stop()
        client.disconnect()
        print("PLC模拟器已停止")

if __name__ == "__main__":
    simulate_plc()
