package influxdb

import (
	"context"
	"fmt"
	"log"
	"time"

	influxdb1 "github.com/influxdata/influxdb1-client/v2"
	"sewage-plant-system/pkg/models"
)

type Client struct {
	client          influxdb1.Client
	database        string
	retentionPolicy string
	timeout         time.Duration
}

func New(addr, username, password, database, retentionPolicy string, timeout time.Duration) (*Client, error) {
	c, err := influxdb1.NewHTTPClient(influxdb1.HTTPConfig{
		Addr:     addr,
		Username: username,
		Password: password,
		Timeout:  timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create influxdb client: %w", err)
	}

	_, _, err = c.Ping(timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to ping influxdb: %w", err)
	}

	return &Client{
		client:          c,
		database:        database,
		retentionPolicy: retentionPolicy,
		timeout:         timeout,
	}, nil
}

func (c *Client) WriteSensorData(data *models.SensorData) error {
	points, err := influxdb1.NewPoints(
		[]influxdb1.Point{
			influxdb1.NewPointWithMeasurement("sensor_data").
				AddTag("sensor_id", data.SensorID).
				AddTag("type", string(data.Type)).
				AddTag("location", data.Location).
				AddTag("status", data.Status).
				AddField("value", data.Value).
				AddField("unit", data.Unit).
				SetTime(data.Timestamp),
		})
	if err != nil {
		return fmt.Errorf("failed to create points: %w", err)
	}

	bp, err := influxdb1.NewBatchPoints(influxdb1.BatchPointsConfig{
		Database:        c.database,
		RetentionPolicy: c.retentionPolicy,
		Precision:       "s",
	})
	if err != nil {
		return fmt.Errorf("failed to create batch points: %w", err)
	}

	bp.AddPoints(points)
	return c.client.Write(bp)
}

func (c *Client) WriteEnergyData(energyType string, value float64, timestamp time.Time) error {
	point, err := influxdb1.NewPoint(
		"energy_data",
		map[string]string{"type": energyType},
		map[string]interface{}{"value": value},
		timestamp,
	)
	if err != nil {
		return err
	}

	bp, err := influxdb1.NewBatchPoints(influxdb1.BatchPointsConfig{
		Database:        c.database,
		RetentionPolicy: c.retentionPolicy,
	})
	if err != nil {
		return err
	}

	bp.AddPoint(point)
	return c.client.Write(bp)
}

func (c *Client) WriteControlCommand(cmd *models.ControlCommand) error {
	point, err := influxdb1.NewPoint(
		"control_commands",
		map[string]string{
			"command_id":  cmd.CommandID,
			"target_type": cmd.TargetType,
			"target_id":   cmd.TargetID,
			"action":      cmd.Action,
			"source":      cmd.Source,
		},
		map[string]interface{}{"value": cmd.Value, "unit": cmd.Unit},
		cmd.Timestamp,
	)
	if err != nil {
		return err
	}

	bp, err := influxdb1.NewBatchPoints(influxdb1.BatchPointsConfig{Database: c.database})
	if err != nil {
		return err
	}

	bp.AddPoint(point)
	return c.client.Write(bp)
}

func (c *Client) WritePLCStatus(status *models.PLCStatus) error {
	point, err := influxdb1.NewPoint(
		"plc_status",
		map[string]string{
			"plc_id":      status.PLCID,
			"device_type": status.DeviceType,
			"device_id":   status.DeviceID,
			"status":      status.Status,
			"fault_code":  status.FaultCode,
		},
		map[string]interface{}{"value": status.Value},
		status.Timestamp,
	)
	if err != nil {
		return err
	}

	bp, err := influxdb1.NewBatchPoints(influxdb1.BatchPointsConfig{Database: c.database})
	if err != nil {
		return err
	}

	bp.AddPoint(point)
	return c.client.Write(bp)
}

func (c *Client) WriteAlert(alert *models.Alert) error {
	point, err := influxdb1.NewPoint(
		"alerts",
		map[string]string{
			"alert_id":   alert.AlertID,
			"level":      fmt.Sprintf("%d", alert.Level),
			"type":       alert.Type,
			"sensor_id":  alert.SensorID,
			"acknowledged": fmt.Sprintf("%t", alert.Acknowledged),
		},
		map[string]interface{}{
			"title":     alert.Title,
			"message":   alert.Message,
			"value":     alert.Value,
			"threshold": alert.Threshold,
		},
		alert.Timestamp,
	)
	if err != nil {
		return err
	}

	bp, err := influxdb1.NewBatchPoints(influxdb1.BatchPointsConfig{Database: c.database})
	if err != nil {
		return err
	}

	bp.AddPoint(point)
	return c.client.Write(bp)
}

func (c *Client) QuerySensorTrend(sensorID string, start, end time.Time) (*models.TrendData, error) {
	query := fmt.Sprintf(`SELECT time, value FROM "sensor_data" WHERE sensor_id = '%s' AND time >= '%s' AND time <= '%s' ORDER BY time ASC`,
		sensorID, start.Format(time.RFC3339), end.Format(time.RFC3339))

	resp, err := c.client.Query(influxdb1.Query{Command: query, Database: c.database})
	if err != nil {
		return nil, err
	}

	if resp.Error() != nil {
		return nil, resp.Error()
	}

	trend := &models.TrendData{
		Timestamps: make([]time.Time, 0),
		Values:     make([]float64, 0),
	}

	for _, result := range resp.Results {
		for _, row := range result.Series {
			for _, value := range row.Values {
				t, _ := time.Parse(time.RFC3339, value[0].(string))
				v, _ := value[1].(float64)
				trend.Timestamps = append(trend.Timestamps, t)
				trend.Values = append(trend.Values, v)
			}
		}
	}

	return trend, nil
}

func (c *Client) QueryLatestSensorData(sensorID string) (float64, time.Time, error) {
	query := fmt.Sprintf(`SELECT last(value), time FROM "sensor_data" WHERE sensor_id = '%s' LIMIT 1`, sensorID)

	resp, err := c.client.Query(influxdb1.Query{Command: query, Database: c.database})
	if err != nil {
		return 0, time.Time{}, err
	}

	if resp.Error() != nil {
		return 0, time.Time{}, resp.Error()
	}

	for _, result := range resp.Results {
		for _, row := range result.Series {
			for _, value := range row.Values {
				t, _ := time.Parse(time.RFC3339, value[0].(string))
				v, _ := value[1].(float64)
				return v, t, nil
			}
		}
	}

	return 0, time.Time{}, fmt.Errorf("no data found for sensor %s", sensorID)
}

func (c *Client) QuerySensorsByType(sensorType models.SensorType, start, end time.Time) (map[string]*models.TrendData, error) {
	query := fmt.Sprintf(`SELECT time, value, sensor_id FROM "sensor_data" WHERE type = '%s' AND time >= '%s' AND time <= '%s' GROUP BY sensor_id ORDER BY time ASC`,
		sensorType, start.Format(time.RFC3339), end.Format(time.RFC3339))

	resp, err := c.client.Query(influxdb1.Query{Command: query, Database: c.database})
	if err != nil {
		return nil, err
	}

	if resp.Error() != nil {
		return nil, resp.Error()
	}

	result := make(map[string]*models.TrendData)

	for _, res := range resp.Results {
		for _, row := range res.Series {
			sensorID := row.Tags["sensor_id"]
			if _, ok := result[sensorID]; !ok {
				result[sensorID] = &models.TrendData{
					Timestamps: make([]time.Time, 0),
					Values:     make([]float64, 0),
				}
			}
			for _, value := range row.Values {
				t, _ := time.Parse(time.RFC3339, value[0].(string))
				v, _ := value[1].(float64)
				result[sensorID].Timestamps = append(result[sensorID].Timestamps, t)
				result[sensorID].Values = append(result[sensorID].Values, v)
			}
		}
	}

	return result, nil
}

func (c *Client) QueryKPI(start, end time.Time) ([]*models.KPIData, error) {
	query := `SELECT time, energy_per_ton, carbon_per_ton, nh3_removal_rate, tn_removal_rate, tp_removal_rate 
		FROM "kpi_data" WHERE time >= $start AND time <= $end ORDER BY time ASC`

	resp, err := c.client.Query(influxdb1.Query{
		Command:  query,
		Database: c.database,
		Params:   map[string]interface{}{"start": start, "end": end},
	})
	if err != nil {
		return nil, err
	}

	if resp.Error() != nil {
		return nil, resp.Error()
	}

	kpis := make([]*models.KPIData, 0)

	for _, res := range resp.Results {
		for _, row := range res.Series {
			for _, value := range row.Values {
				t, _ := time.Parse(time.RFC3339, value[0].(string))
				kpi := &models.KPIData{
					Timestamp:      t,
					EnergyPerTon:   getFloat(value[1]),
					CarbonPerTon:   getFloat(value[2]),
					NH3RemovalRate: getFloat(value[3]),
					TNRemovalRate:  getFloat(value[4]),
					TPRemovalRate:  getFloat(value[5]),
				}
				kpis = append(kpis, kpi)
			}
		}
	}

	return kpis, nil
}

func (c *Client) WriteKPI(kpi *models.KPIData) error {
	point, err := influxdb1.NewPoint(
		"kpi_data",
		nil,
		map[string]interface{}{
			"energy_per_ton":   kpi.EnergyPerTon,
			"carbon_per_ton":   kpi.CarbonPerTon,
			"nh3_removal_rate": kpi.NH3RemovalRate,
			"tn_removal_rate":  kpi.TNRemovalRate,
			"tp_removal_rate":  kpi.TPRemovalRate,
			"water_quality":    kpi.WaterQuality,
		},
		kpi.Timestamp,
	)
	if err != nil {
		return err
	}

	bp, err := influxdb1.NewBatchPoints(influxdb1.BatchPointsConfig{Database: c.database})
	if err != nil {
		return err
	}

	bp.AddPoint(point)
	return c.client.Write(bp)
}

func (c *Client) QueryAlerts(start, end time.Time, level int) ([]*models.Alert, error) {
	query := fmt.Sprintf(`SELECT * FROM "alerts" WHERE time >= '%s' AND time <= '%s'`,
		start.Format(time.RFC3339), end.Format(time.RFC3339))
	if level > 0 {
		query += fmt.Sprintf(` AND level = '%d'`, level)
	}
	query += ` ORDER BY time DESC LIMIT 100`

	resp, err := c.client.Query(influxdb1.Query{Command: query, Database: c.database})
	if err != nil {
		return nil, err
	}

	if resp.Error() != nil {
		return nil, resp.Error()
	}

	alerts := make([]*models.Alert, 0)

	for _, res := range resp.Results {
		for _, row := range res.Series {
			for _, value := range row.Values {
				t, _ := time.Parse(time.RFC3339, value[0].(string))
				alert := &models.Alert{
					Timestamp: t,
				}
				for i, col := range row.Columns {
					switch col {
					case "alert_id":
						alert.AlertID = getString(value[i])
					case "level":
						alert.Level = getInt(value[i])
					case "type":
						alert.Type = getString(value[i])
					case "title":
						alert.Title = getString(value[i])
					case "message":
						alert.Message = getString(value[i])
					case "sensor_id":
						alert.SensorID = getString(value[i])
					case "value":
						alert.Value = getFloat(value[i])
					case "threshold":
						alert.Threshold = getFloat(value[i])
					case "acknowledged":
						alert.Acknowledged = getBool(value[i])
					}
				}
				alerts = append(alerts, alert)
			}
		}
	}

	return alerts, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

func getFloat(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

func getString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func getInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		var i int
		fmt.Sscanf(val, "%d", &i)
		return i
	default:
		return 0
	}
}

func getBool(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "1"
	case int:
		return val == 1
	default:
		return false
	}
}

func (c *Client) WriteAerationControl(ac *models.AerationControl, timestamp time.Time) error {
	point, err := influxdb1.NewPoint(
		"aeration_control",
		map[string]string{"zone_id": ac.ZoneID},
		map[string]interface{}{
			"do_actual":            ac.DOActual,
			"do_setpoint":          ac.DOSetpoint,
			"nh3_actual":           ac.NH3Actual,
			"nh3_setpoint":         ac.NH3Setpoint,
			"air_flow_setpoint":    ac.AirFlowSetpoint,
			"valve_opening":        ac.ValveOpening,
			"fan_speed":            ac.FanSpeed,
			"pid_output":           ac.PIDOutput,
			"feedforward_output":   ac.FeedforwardOutput,
			"total_output":         ac.TotalOutput,
		},
		timestamp,
	)
	if err != nil {
		return err
	}

	bp, err := influxdb1.NewBatchPoints(influxdb1.BatchPointsConfig{Database: c.database})
	if err != nil {
		return err
	}

	bp.AddPoint(point)
	return c.client.Write(bp)
}

func (c *Client) WriteCarbonControl(cc *models.CarbonControl, timestamp time.Time) error {
	point, err := influxdb1.NewPoint(
		"carbon_control",
		nil,
		map[string]interface{}{
			"no3_actual":         cc.NO3Actual,
			"cod_influent":       cc.CODInfluent,
			"tn_estimate":        cc.TNEstimate,
			"dosage_setpoint":    cc.DosageSetpoint,
			"carbon_source_type": cc.CarbonSourceType,
			"removal_rate":       cc.RemovalRate,
		},
		timestamp,
	)
	if err != nil {
		return err
	}

	bp, err := influxdb1.NewBatchPoints(influxdb1.BatchPointsConfig{Database: c.database})
	if err != nil {
		return err
	}

	bp.AddPoint(point)
	return c.client.Write(bp)
}

func (c *Client) QuerySensorStatus(sensorType models.SensorType, timeoutSeconds int) (map[string]time.Time, error) {
	query := fmt.Sprintf(`SELECT last(value), time, sensor_id FROM "sensor_data" WHERE type = '%s' GROUP BY sensor_id`, sensorType)

	resp, err := c.client.Query(influxdb1.Query{Command: query, Database: c.database})
	if err != nil {
		return nil, err
	}

	if resp.Error() != nil {
		return nil, resp.Error()
	}

	status := make(map[string]time.Time)

	for _, res := range resp.Results {
		for _, row := range res.Series {
			sensorID := row.Tags["sensor_id"]
			for _, value := range row.Values {
				t, _ := time.Parse(time.RFC3339, value[0].(string))
				status[sensorID] = t
			}
		}
	}

	return status, nil
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func (c *Client) GetDatabase() string {
	return c.database
}

func (c *Client) GetRetentionPolicy() string {
	return c.retentionPolicy
}

func (c *Client) GetClient() influxdb1.Client {
	return c.client
}

var (
	_ context.Context
)
