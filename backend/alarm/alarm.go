package alarm

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"sewage-treatment/backend/config"
	"sewage-treatment/backend/influxdb"
	"sewage-treatment/backend/models"
)

type AlarmState struct {
	Level1ExceedStartTime map[string]time.Time
	Level2OfflineStartTime map[string]time.Time
	ActiveAlarms          map[string]*models.Alarm
	mu                    sync.RWMutex
}

type AlarmManager struct {
	state      *AlarmState
	running    bool
	stopChan   chan struct{}
	alarmChan  chan *models.Alarm
	wsPushFunc func(*models.Alarm)
}

var AlarmMgr *AlarmManager
var alarmHistory []*models.Alarm
var alarmHistoryMu sync.RWMutex

func NewAlarmManager(wsPushFunc func(*models.Alarm)) *AlarmManager {
	return &AlarmManager{
		state: &AlarmState{
			Level1ExceedStartTime: make(map[string]time.Time),
			Level2OfflineStartTime: make(map[string]time.Time),
			ActiveAlarms:          make(map[string]*models.Alarm),
		},
		stopChan:   make(chan struct{}),
		alarmChan:  make(chan *models.Alarm, 100),
		wsPushFunc: wsPushFunc,
	}
}

func (am *AlarmManager) Start() {
	if am.running {
		return
	}
	am.running = true

	ticker := time.NewTicker(1 * time.Minute)

	go func() {
		for {
			select {
			case <-ticker.C:
				am.checkAlarms()
			case <-am.stopChan:
				ticker.Stop()
				am.running = false
				return
			}
		}
	}()

	go am.processAlarms()

	log.Println("Alarm manager started")
}

func (am *AlarmManager) Stop() {
	if !am.running {
		return
	}
	am.stopChan <- struct{}{}
	log.Println("Alarm manager stopped")
}

func (am *AlarmManager) checkAlarms() {
	am.checkLevel1Alarms()
	am.checkLevel2Alarms()
}

func (am *AlarmManager) checkLevel1Alarms() {
	level1Config := config.AppConfig.Alarm.Level1

	nh3Data, err := influxdb.InfluxClient.QueryLatestSensorData("NH3-EFF")
	if err == nil {
		am.checkSingleLevel1Alarm("NH3-EFF", nh3Data.Value, level1Config.NH3Threshold, "氨氮")
	}

	tnData, err := influxdb.InfluxClient.QueryLatestSensorData("TN-EFF")
	if err == nil {
		am.checkSingleLevel1Alarm("TN-EFF", tnData.Value, level1Config.TNThreshold, "总氮")
	}
}

func (am *AlarmManager) checkSingleLevel1Alarm(sensorID string, value, threshold float64, name string) {
	am.state.mu.Lock()
	defer am.state.mu.Unlock()

	durationThreshold := time.Duration(config.AppConfig.Alarm.Level1.DurationMinutes) * time.Minute

	if value > threshold {
		if _, exists := am.state.Level1ExceedStartTime[sensorID]; !exists {
			am.state.Level1ExceedStartTime[sensorID] = time.Now()
		} else {
			exceedDuration := time.Since(am.state.Level1ExceedStartTime[sensorID])
			if exceedDuration >= durationThreshold {
				alarmKey := fmt.Sprintf("level1_%s", sensorID)
				if _, active := am.state.ActiveAlarms[alarmKey]; !active {
					alarm := &models.Alarm{
						ID:        fmt.Sprintf("alarm_%d", time.Now().UnixNano()),
						Level:     1,
						Type:      "effluent_exceed",
						Message:   fmt.Sprintf("出水%s超标: %.2f mg/L, 超过阈值 %.2f mg/L, 已持续 %d 分钟",
							name, value, threshold, int(exceedDuration.Minutes())),
						SensorID:  sensorID,
						Value:     value,
						Threshold: threshold,
						Timestamp: time.Now(),
						AckStatus: false,
					}
					am.state.ActiveAlarms[alarmKey] = alarm
					am.alarmChan <- alarm
				}
			}
		}
	} else {
		delete(am.state.Level1ExceedStartTime, sensorID)
		alarmKey := fmt.Sprintf("level1_%s", sensorID)
		if alarm, active := am.state.ActiveAlarms[alarmKey]; active {
			alarm.AckStatus = true
			alarm.Message += " (已恢复)"
			delete(am.state.ActiveAlarms, alarmKey)
		}
	}
}

func (am *AlarmManager) checkLevel2Alarms() {
	am.checkBlowerStatus()
	am.checkSensorOffline()
}

func (am *AlarmManager) checkBlowerStatus() {
	am.state.mu.Lock()
	defer am.state.mu.Unlock()

	offlineMinutes := config.AppConfig.Alarm.Level2.OfflineMinutes
	offlineDuration := time.Duration(offlineMinutes) * time.Minute

	for i := 1; i <= 3; i++ {
		blowerID := fmt.Sprintf("blower_%d", i)
		alarmKey := fmt.Sprintf("level2_blower_%d", i)

		status := am.getBlowerStatus(i)

		if !status {
			if _, exists := am.state.Level2OfflineStartTime[blowerID]; !exists {
				am.state.Level2OfflineStartTime[blowerID] = time.Now()
			} else {
				offlineTime := time.Since(am.state.Level2OfflineStartTime[blowerID])
				if offlineTime >= offlineDuration {
					if _, active := am.state.ActiveAlarms[alarmKey]; !active {
						alarm := &models.Alarm{
							ID:        fmt.Sprintf("alarm_%d", time.Now().UnixNano()),
							Level:     2,
							Type:      "equipment_fault",
							Message:   fmt.Sprintf("曝气风机 %d 故障, 已离线 %d 分钟", i, int(offlineTime.Minutes())),
							SensorID:  blowerID,
							Value:     0,
							Threshold: 1,
							Timestamp: time.Now(),
							AckStatus: false,
						}
						am.state.ActiveAlarms[alarmKey] = alarm
						am.alarmChan <- alarm
					}
				}
			}
		} else {
			delete(am.state.Level2OfflineStartTime, blowerID)
			if alarm, active := am.state.ActiveAlarms[alarmKey]; active {
				alarm.AckStatus = true
				alarm.Message += " (已恢复)"
				delete(am.state.ActiveAlarms, alarmKey)
			}
		}
	}
}

func (am *AlarmManager) getBlowerStatus(blowerNum int) bool {
	start := time.Now().Add(-2 * time.Minute)
	end := time.Now()

	rows, err := influxdb.InfluxClient.QueryAverageByTimeRange(
		"control_data", start, end, "1m",
	)
	if err != nil || len(rows) == 0 {
		return true
	}

	for _, row := range rows {
		if row.Tags["target"] == fmt.Sprintf("blower_%d", blowerNum) {
			return true
		}
	}

	return true
}

func (am *AlarmManager) checkSensorOffline() {
	am.state.mu.Lock()
	defer am.state.mu.Unlock()

	offlineMinutes := config.AppConfig.Alarm.Level2.OfflineMinutes
	offlineDuration := time.Duration(offlineMinutes) * time.Minute

	allSensors, err := influxdb.InfluxClient.QueryAllLatestSensorData()
	if err != nil {
		return
	}

	sensorMap := make(map[string]*models.SensorData)
	for i := range allSensors {
		sensorMap[allSensors[i].ID] = &allSensors[i]
	}

	for _, cfg := range models.SensorConfigs {
		if cfg.Type != models.SensorTypeDO {
			continue
		}

		sensorID := cfg.ID
		alarmKey := fmt.Sprintf("level2_sensor_%s", sensorID)

		data, exists := sensorMap[sensorID]
		online := false
		if exists {
			if time.Since(data.Timestamp) < offlineDuration {
				online = true
			}
		}

		if !online {
			if _, offlineStart := am.state.Level2OfflineStartTime[sensorID]; !offlineStart {
				am.state.Level2OfflineStartTime[sensorID] = time.Now()
			} else {
				offlineTime := time.Since(am.state.Level2OfflineStartTime[sensorID])
				if offlineTime >= offlineDuration {
					if _, active := am.state.ActiveAlarms[alarmKey]; !active {
						alarm := &models.Alarm{
							ID:        fmt.Sprintf("alarm_%d", time.Now().UnixNano()),
							Level:     2,
							Type:      "sensor_offline",
							Message:   fmt.Sprintf("DO传感器 %s 离线, 已离线 %d 分钟", sensorID, int(offlineTime.Minutes())),
							SensorID:  sensorID,
							Value:     0,
							Threshold: 1,
							Timestamp: time.Now(),
							AckStatus: false,
						}
						am.state.ActiveAlarms[alarmKey] = alarm
						am.alarmChan <- alarm
					}
				}
			}
		} else {
			delete(am.state.Level2OfflineStartTime, sensorID)
			if alarm, active := am.state.ActiveAlarms[alarmKey]; active {
				alarm.AckStatus = true
				alarm.Message += " (已恢复)"
				delete(am.state.ActiveAlarms, alarmKey)
			}
		}
	}
}

func (am *AlarmManager) processAlarms() {
	for alarm := range am.alarmChan {
		if err := influxdb.InfluxClient.WriteAlarm(alarm); err != nil {
			log.Printf("Failed to write alarm to InfluxDB: %v", err)
		}

		am.sendSMSNotification(alarm)

		if am.wsPushFunc != nil {
			am.wsPushFunc(alarm)
		}

		alarmHistoryMu.Lock()
		alarmHistory = append(alarmHistory, alarm)
		if len(alarmHistory) > 1000 {
			alarmHistory = alarmHistory[len(alarmHistory)-1000:]
		}
		alarmHistoryMu.Unlock()

		log.Printf("ALARM [Level %d]: %s", alarm.Level, alarm.Message)
	}
}

func (am *AlarmManager) sendSMSNotification(alarm *models.Alarm) {
	smsConfig := config.AppConfig.Alarm.SMS
	if len(smsConfig.Phones) == 0 {
		log.Printf("SMS notification skipped: no phone numbers configured")
		return
	}

	message := fmt.Sprintf("[污水处理告警] 等级%d: %s", alarm.Level, alarm.Message)

	for _, phone := range smsConfig.Phones {
		log.Printf("Sending SMS to %s: %s", phone, message)

		go func(p, m string) {
			if err := sendSMS(p, m, smsConfig); err != nil {
				log.Printf("Failed to send SMS to %s: %v", p, err)
			} else {
				log.Printf("SMS sent successfully to %s", p)
			}
		}(phone, message)
	}
}

func sendSMS(phone, message string, config config.SMSConfig) error {
	log.Printf("[SMS SIMULATION] To: %s, Message: %s", phone, message)
	return nil
}

func (am *AlarmManager) GetActiveAlarms() []*models.Alarm {
	am.state.mu.RLock()
	defer am.state.mu.RUnlock()

	alarms := make([]*models.Alarm, 0, len(am.state.ActiveAlarms))
	for _, alarm := range am.state.ActiveAlarms {
		alarms = append(alarms, alarm)
	}
	return alarms
}

func (am *AlarmManager) AcknowledgeAlarm(alarmID string) error {
	am.state.mu.Lock()
	defer am.state.mu.Unlock()

	for key, alarm := range am.state.ActiveAlarms {
		if alarm.ID == alarmID {
			alarm.AckStatus = true
			delete(am.state.ActiveAlarms, key)
			log.Printf("Alarm %s acknowledged", alarmID)
			return nil
		}
	}
	return fmt.Errorf("alarm not found: %s", alarmID)
}

func GetAlarmHistory(limit int) []*models.Alarm {
	alarmHistoryMu.RLock()
	defer alarmHistoryMu.RUnlock()

	if limit <= 0 || limit > len(alarmHistory) {
		limit = len(alarmHistory)
	}

	start := len(alarmHistory) - limit
	if start < 0 {
		start = 0
	}

	return alarmHistory[start:]
}

func BroadcastSensorUpdate(data *models.SensorData) {
	if AlarmMgr == nil || AlarmMgr.wsPushFunc == nil {
		return
	}

	updateMsg := map[string]interface{}{
		"type":      "sensor_update",
		"sensor_id": data.ID,
		"value":     data.Value,
		"timestamp": data.Timestamp,
		"alarm_level": data.AlarmLevel,
	}

	jsonData, _ := json.Marshal(updateMsg)
	log.Printf("Broadcast sensor update: %s", string(jsonData))
}
