package influxdb

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/influxdata/influxdb1-client/models"
	client "github.com/influxdata/influxdb1-client/v2"

	"sewage-treatment/backend/config"
	"sewage-treatment/backend/models"
)

type Client struct {
	cli client.Client
	db  string
}

var InfluxClient *Client

func NewClient() error {
	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     config.AppConfig.InfluxDB.Addr,
		Username: config.AppConfig.InfluxDB.Username,
		Password: config.AppConfig.InfluxDB.Password,
	})
	if err != nil {
		return err
	}

	InfluxClient = &Client{
		cli: c,
		db:  config.AppConfig.InfluxDB.Database,
	}

	_, _, err = c.Ping(5 * time.Second)
	if err != nil {
		return fmt.Errorf("failed to ping InfluxDB: %v", err)
	}

	log.Println("Connected to InfluxDB successfully")
	return nil
}

func (c *Client) WriteSensorData(data *models.SensorData) error {
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:        c.db,
		RetentionPolicy: "raw_data",
		Precision:     "s",
	})
	if err != nil {
		return err
	}

	tags := map[string]string{
		"id":    data.ID,
		"type":  string(data.Type),
		"stage": string(data.Stage),
		"section": fmt.Sprintf("%d", data.Section),
		"unit":  data.Unit,
	}

	fields := map[string]interface{}{
		"value":       data.Value,
		"status":      data.Status,
		"alarm_level": data.AlarmLevel,
	}

	pt, err := client.NewPoint("sensor_data", tags, fields, data.Timestamp)
	if err != nil {
		return err
	}

	bp.AddPoint(pt)
	return c.cli.Write(bp)
}

func (c *Client) WriteControlCommand(cmd *models.ControlCommand) error {
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:        c.db,
		RetentionPolicy: "raw_data",
		Precision:     "s",
	})
	if err != nil {
		return err
	}

	tags := map[string]string{
		"id":     cmd.ID,
		"type":   cmd.Type,
		"target": cmd.Target,
		"unit":   cmd.Unit,
		"source": cmd.Source,
	}

	fields := map[string]interface{}{
		"value": cmd.Value,
	}

	pt, err := client.NewPoint("control_data", tags, fields, cmd.Timestamp)
	if err != nil {
		return err
	}

	bp.AddPoint(pt)
	return c.cli.Write(bp)
}

func (c *Client) WriteAlarm(alarm *models.Alarm) error {
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:        c.db,
		RetentionPolicy: "raw_data",
		Precision:     "s",
	})
	if err != nil {
		return err
	}

	tags := map[string]string{
		"id":        alarm.ID,
		"level":     fmt.Sprintf("%d", alarm.Level),
		"type":      alarm.Type,
		"sensor_id": alarm.SensorID,
	}

	fields := map[string]interface{}{
		"value":        alarm.Value,
		"threshold":   alarm.Threshold,
		"message":    alarm.Message,
		"acknowledged": alarm.AckStatus,
	}

	pt, err := client.NewPoint("alarm_data", tags, fields, alarm.Timestamp)
	if err != nil {
		return err
	}

	bp.AddPoint(pt)
	return c.cli.Write(bp)
}

func (c *Client) WriteKPI(kpi *models.KPIData) error {
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:        c.db,
		RetentionPolicy: "raw_data",
		Precision:     "s",
	})
	if err != nil {
		return err
	}

	tags := map[string]string{
		"id":   kpi.ID,
		"type": kpi.Type,
		"unit": kpi.Unit,
	}

	fields := map[string]interface{}{
		"value": kpi.Value,
	}

	pt, err := client.NewPoint("kpi_data", tags, fields, kpi.Timestamp)
	if err != nil {
		return err
	}

	bp.AddPoint(pt)
	return c.cli.Write(bp)
}

func (c *Client) QuerySensorTrend(sensorID string, start, end time.Time) ([]models.TrendPoint, error) {
	query := fmt.Sprintf(`SELECT mean("value") as value FROM "raw_data"."sensor_data" WHERE "id" = '%s' AND time >= '%s' AND time <= '%s' GROUP BY time(5m) fill(none)`,
		sensorID, start.Format(time.RFC3339), end.Format(time.RFC3339))

	resp, err := c.cli.Query(client.NewQuery(query, c.db, "s"))
	if err != nil {
		return nil, err
	}
	if resp.Error() != nil {
		return nil, resp.Error()
	}

	points := make([]models.TrendPoint, 0)
	for _, result := range resp.Results {
		for _, row := range result.Series {
			for _, v := range row.Values {
				t, _ := time.Parse(time.RFC3339, v[0].(string))
				val, _ := v[1].(float64)
				points = append(points, models.TrendPoint{
					Time:  t,
					Value: val,
				})
			}
		}
	}
	return points, nil
}

func (c *Client) QueryLatestSensorData(sensorID string) (*models.SensorData, error) {
	query := fmt.Sprintf(`SELECT last("value") as value, "status", "alarm_level", "type", "stage", "section", "unit" FROM "raw_data"."sensor_data" WHERE "id" = '%s' GROUP BY "id"`, sensorID)

	resp, err := c.cli.Query(client.NewQuery(query, c.db, "s"))
	if err != nil {
		return nil, err
	}
	if resp.Error() != nil {
		return nil, resp.Error()
	}

	for _, result := range resp.Results {
		for _, row := range result.Series {
			if len(row.Values) > 0 {
				v := row.Values[0]
				t, _ := time.Parse(time.RFC3339, v[0].(string))
				val, _ := v[1].(float64)
				status, _ := v[2].(string)
				alarmLevel, _ := v[3].(int64)
				sensorType, _ := v[4].(string)
				stage, _ := v[5].(string)
				section, _ := v[6].(string)
				unit, _ := v[7].(string)

				sec := 0
				fmt.Sscanf(section, "%d", &sec)

				return &models.SensorData{
					ID:         sensorID,
					Type:       models.SensorType(sensorType),
					Stage:      models.ProcessStage(stage),
					Section:    sec,
					Value:      val,
					Unit:       unit,
					Timestamp:  t,
					Status:     status,
					AlarmLevel: int(alarmLevel),
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("no data found")
}

func (c *Client) QueryKPITrend(kpiType string, start, end time.Time) ([]models.TrendPoint, error) {
	query := fmt.Sprintf(`SELECT mean("value") as value FROM "raw_data"."kpi_data" WHERE "type" = '%s' AND time >= '%s' AND time <= '%s' GROUP BY time(1h) fill(none)`,
		kpiType, start.Format(time.RFC3339), end.Format(time.RFC3339))

	resp, err := c.cli.Query(client.NewQuery(query, c.db, "s"))
	if err != nil {
		return nil, err
	}
	if resp.Error() != nil {
		return nil, resp.Error()
	}

	points := make([]models.TrendPoint, 0)
	for _, result := range resp.Results {
		for _, row := range result.Series {
			for _, v := range row.Values {
				t, _ := time.Parse(time.RFC3339, v[0].(string))
				val, _ := v[1].(float64)
				points = append(points, models.TrendPoint{
					Time:  t,
					Value: val,
				})
			}
		}
	}
	return points, nil
}

func (c *Client) QueryActiveAlarms() ([]models.Alarm, error) {
	query := `SELECT "value", "threshold", "message", "level", "type", "sensor_id", "acknowledged" FROM "raw_data"."alarm_data" WHERE time > now() - 1h AND "acknowledged" = false ORDER BY time DESC`

	resp, err := c.cli.Query(client.NewQuery(query, c.db, "s"))
	if err != nil {
		return nil, err
	}
	if resp.Error() != nil {
		return nil, resp.Error()
	}

	alarms := make([]models.Alarm, 0)
	for _, result := range resp.Results {
		for _, row := range result.Series {
			for _, v := range row.Values {
				t, _ := time.Parse(time.RFC3339, v[0].(string))
				val, _ := v[1].(float64)
				threshold, _ := v[2].(float64)
				msg, _ := v[3].(string)
				level, _ := v[4].(string)
				typ, _ := v[5].(string)
				sensorID, _ := v[6].(string)
				ack, _ := v[7].(bool)

				lvl := 0
				fmt.Sscanf(level, "%d", &lvl)

				alarm := models.Alarm{
					ID:        fmt.Sprintf("%d", t.Unix()),
					Level:     lvl,
					Type:      typ,
					Message:   msg,
					SensorID:  sensorID,
					Value:     val,
					Threshold: threshold,
					Timestamp: t,
					AckStatus:  ack,
				}
				alarms = append(alarms, alarm)
			}
		}
	}
	return alarms, nil
}

func (c *Client) QueryAllLatestSensorData() ([]models.SensorData, error) {
	query := `SELECT last("value") as value, "status", "alarm_level" FROM "raw_data"."sensor_data" WHERE time > now() - 10m GROUP BY "id", "type", "stage", "section", "unit"`

	resp, err := c.cli.Query(client.NewQuery(query, c.db, "s"))
	if err != nil {
		return nil, err
	}
	if resp.Error() != nil {
		return nil, resp.Error()
	}

	dataList := make([]models.SensorData, 0)
	for _, result := range resp.Results {
		for _, row := range result.Series {
			if len(row.Values) > 0 {
				v := row.Values[0]
				t, _ := time.Parse(time.RFC3339, v[0].(string))
				val, _ := v[1].(float64)
				status, _ := v[2].(string)
				alarmLevel, _ := v[3].(int64)

				id := row.Tags["id"]
				sensorType := row.Tags["type"]
				stage := row.Tags["stage"]
				sectionStr := row.Tags["section"]
				unit := row.Tags["unit"]

				sec := 0
				fmt.Sscanf(sectionStr, "%d", &sec)

				dataList = append(dataList, models.SensorData{
					ID:         id,
					Type:       models.SensorType(sensorType),
					Stage:      models.ProcessStage(stage),
					Section:    sec,
					Value:      val,
					Unit:       unit,
					Timestamp:  t,
					Status:     status,
					AlarmLevel: int(alarmLevel),
				})
			}
		}
	}
	return dataList, nil
}

func (c *Client) QuerySensorDataByType(sensorType string, start, end time.Time) ([]models.SensorData, error) {
	query := fmt.Sprintf(`SELECT "value", "status", "alarm_level" FROM "raw_data"."sensor_data" WHERE "type" = '%s' AND time >= '%s' AND time <= '%s' GROUP BY "id"`,
		sensorType, start.Format(time.RFC3339), end.Format(time.RFC3339))

	resp, err := c.cli.Query(client.NewQuery(query, c.db, "s"))
	if err != nil {
		return nil, err
	}
	if resp.Error() != nil {
		return nil, resp.Error()
	}

	dataList := make([]models.SensorData, 0)
	for _, result := range resp.Results {
		for _, row := range result.Series {
			for _, v := range row.Values {
				t, _ := time.Parse(time.RFC3339, v[0].(string))
				val, _ := v[1].(float64)
				status, _ := v[2].(string)
				alarmLevel, _ := v[3].(int64)

				id := row.Tags["id"]
				stage := row.Tags["stage"]
				sectionStr := row.Tags["section"]
				unit := row.Tags["unit"]

				sec := 0
				fmt.Sscanf(sectionStr, "%d", &sec)

				dataList = append(dataList, models.SensorData{
					ID:         id,
					Type:       models.SensorType(sensorType),
					Stage:      models.ProcessStage(stage),
					Section:    sec,
					Value:      val,
					Unit:       unit,
					Timestamp:  t,
					Status:     status,
					AlarmLevel: int(alarmLevel),
				})
			}
		}
	}
	return dataList, nil
}

func (c *Client) QueryAverageByTimeRange(measurement string, start, end time.Time, groupBy string) ([]models.Row, error) {
	query := fmt.Sprintf(`SELECT mean("value") as value FROM "raw_data".%s WHERE time >= '%s' AND time <= '%s' GROUP BY time(%s) fill(none)`,
		measurement, start.Format(time.RFC3339), end.Format(time.RFC3339), groupBy)

	resp, err := c.cli.Query(client.NewQuery(query, c.db, "s"))
	if err != nil {
		return nil, err
	}
	if resp.Error() != nil {
		return nil, resp.Error()
	}

	rows := make([]models.Row, 0)
	for _, result := range resp.Results {
		for _, series := range result.Series {
			row := models.Row{
				Name:    series.Name,
				Tags:    series.Tags,
				Columns: series.Columns,
				Values:  series.Values,
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (c *Client) Close() error {
	return c.cli.Close()
}
