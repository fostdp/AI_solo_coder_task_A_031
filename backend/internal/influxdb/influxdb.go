package influxdb

import (
	"fmt"
	"time"

	influxdbclient "github.com/influxdata/influxdb1-client/v2"
	"go.uber.org/zap"

	"sewage-treatment-system/internal/config"
	"sewage-treatment-system/internal/models"
)

type Client struct {
	client influxdbclient.Client
	logger *zap.Logger
}

func New(cfg *config.InfluxDBConfig, logger *zap.Logger) (*Client, error) {
	c, err := influxdbclient.NewHTTPClient(influxdbclient.HTTPConfig{
		Addr:     cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
		Timeout:  time.Second * 30,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create influxdb client: %w", err)
	}

	_, _, err = c.Ping(time.Second * 5)
	if err != nil {
		return nil, fmt.Errorf("failed to ping influxdb: %w", err)
	}

	return &Client{client: c, logger: logger}, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) WriteSensorData(data *models.SensorData) error {
	bp, err := influxdbclient.NewBatchPoints(influxdbclient.BatchPointsConfig{
		Database:        config.AppConfig.InfluxDB.Database,
		RetentionPolicy: config.AppConfig.InfluxDB.RetentionPolicy,
		Precision:       config.AppConfig.InfluxDB.Precision,
	})
	if err != nil {
		return err
	}

	tags := map[string]string{
		"id":      data.ID,
		"type":    string(data.Type),
		"stage":   string(data.Stage),
		"section": fmt.Sprintf("%d", data.Section),
		"dtu_id":  data.DTUID,
	}

	fields := map[string]interface{}{
		"value":    data.Value,
		"setpoint": data.Setpoint,
		"status":   data.Status,
	}

	pt, err := influxdbclient.NewPoint("sensor_data", tags, fields, data.Timestamp)
	if err != nil {
		return err
	}

	bp.AddPoint(pt)
	return c.client.Write(bp)
}

func (c *Client) WriteAerationControl(ctrl *models.AerationControl) error {
	bp, err := influxdbclient.NewBatchPoints(influxdbclient.BatchPointsConfig{
		Database:        config.AppConfig.InfluxDB.Database,
		RetentionPolicy: config.AppConfig.InfluxDB.RetentionPolicy,
		Precision:       config.AppConfig.InfluxDB.Precision,
	})
	if err != nil {
		return err
	}

	tags := map[string]string{
		"section": fmt.Sprintf("%d", ctrl.Section),
	}

	fields := map[string]interface{}{
		"air_flow_set":    ctrl.AirFlowSet,
		"air_flow_actual": ctrl.AirFlowActual,
		"valve_open":      ctrl.ValveOpen,
		"do_actual":       ctrl.DOActual,
		"nh3_actual":      ctrl.NH3Actual,
	}

	pt, err := influxdbclient.NewPoint("aeration_control", tags, fields, ctrl.Timestamp)
	if err != nil {
		return err
	}

	bp.AddPoint(pt)
	return c.client.Write(bp)
}

func (c *Client) WriteCarbonDosing(dosing *models.CarbonDosing) error {
	bp, err := influxdbclient.NewBatchPoints(influxdbclient.BatchPointsConfig{
		Database:        config.AppConfig.InfluxDB.Database,
		RetentionPolicy: config.AppConfig.InfluxDB.RetentionPolicy,
		Precision:       config.AppConfig.InfluxDB.Precision,
	})
	if err != nil {
		return err
	}

	tags := map[string]string{}

	fields := map[string]interface{}{
		"dosing_rate":   dosing.DosingRate,
		"dosing_actual": dosing.DosingActual,
		"cod_influent":  dosing.CODInfluent,
		"no3_anoxic":    dosing.NO3Anoxic,
		"tn_removal":    dosing.TNRemoval,
	}

	pt, err := influxdbclient.NewPoint("carbon_dosing", tags, fields, dosing.Timestamp)
	if err != nil {
		return err
	}

	bp.AddPoint(pt)
	return c.client.Write(bp)
}

func (c *Client) WriteKeyMetrics(metrics *models.KeyMetrics) error {
	bp, err := influxdbclient.NewBatchPoints(influxdbclient.BatchPointsConfig{
		Database:        config.AppConfig.InfluxDB.Database,
		RetentionPolicy: config.AppConfig.InfluxDB.RetentionPolicy,
		Precision:       config.AppConfig.InfluxDB.Precision,
	})
	if err != nil {
		return err
	}

	tags := map[string]string{}

	fields := map[string]interface{}{
		"power_consumption": metrics.PowerConsumption,
		"carbon_usage":      metrics.CarbonUsage,
		"tn_removal_rate":   metrics.TNRemovalRate,
		"tp_removal_rate":   metrics.TPRemovalRate,
		"cod_removal_rate":  metrics.CODRemovalRate,
		"flow_rate":         metrics.FlowRate,
	}

	pt, err := influxdbclient.NewPoint("key_metrics", tags, fields, metrics.Timestamp)
	if err != nil {
		return err
	}

	bp.AddPoint(pt)
	return c.client.Write(bp)
}

func (c *Client) WriteAlarm(alarm *models.Alarm) error {
	bp, err := influxdbclient.NewBatchPoints(influxdbclient.BatchPointsConfig{
		Database:        config.AppConfig.InfluxDB.Database,
		RetentionPolicy: config.AppConfig.InfluxDB.RetentionPolicy,
		Precision:       config.AppConfig.InfluxDB.Precision,
	})
	if err != nil {
		return err
	}

	tags := map[string]string{
		"level": fmt.Sprintf("%d", alarm.Level),
		"type":  alarm.Type,
	}

	fields := map[string]interface{}{
		"id":        alarm.ID,
		"message":   alarm.Message,
		"value":     alarm.Value,
		"threshold": alarm.Threshold,
		"ack":       alarm.ACK,
	}

	pt, err := influxdbclient.NewPoint("alarms", tags, fields, alarm.Timestamp)
	if err != nil {
		return err
	}

	bp.AddPoint(pt)
	return c.client.Write(bp)
}

func (c *Client) QuerySensorTrend(sensorID string, duration time.Duration) ([]models.TrendDataPoint, error) {
	query := fmt.Sprintf(`
		SELECT value FROM sensor_data
		WHERE id = '%s' AND time > now() - %dh
		ORDER BY time ASC
	`, sensorID, int(duration.Hours()))

	return c.queryTrend(query)
}

func (c *Client) QueryMetricsTrend(metric string, duration time.Duration) ([]models.TrendDataPoint, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM key_metrics
		WHERE time > now() - %dd
		ORDER BY time ASC
	`, metric, int(duration.Hours()/24))

	return c.queryTrend(query)
}

func (c *Client) QueryLatestSensor(sensorID string) (*models.SensorData, error) {
	query := fmt.Sprintf(`
		SELECT value, setpoint, status FROM sensor_data
		WHERE id = '%s'
		ORDER BY time DESC
		LIMIT 1
	`, sensorID)

	q := influxdbclient.Query{
		Command:  query,
		Database: config.AppConfig.InfluxDB.Database,
	}

	resp, err := c.client.Query(q)
	if err != nil {
		return nil, err
	}
	if resp.Error() != nil {
		return nil, resp.Error()
	}

	if len(resp.Results) > 0 && len(resp.Results[0].Series) > 0 {
		row := resp.Results[0].Series[0]
		if len(row.Values) > 0 {
			value, _ := row.Values[0][1].(float64)
			setpoint, _ := row.Values[0][2].(float64)
			status, _ := row.Values[0][3].(string)
			ts, _ := time.Parse(time.RFC3339, row.Values[0][0].(string))

			return &models.SensorData{
				ID:        sensorID,
				Value:     value,
				Setpoint:  setpoint,
				Status:    status,
				Timestamp: ts,
			}, nil
		}
	}

	return nil, fmt.Errorf("no data found for sensor %s", sensorID)
}

func (c *Client) QueryAllLatestSensors() (map[string]*models.SensorData, error) {
	query := `
		SELECT value, setpoint, status FROM sensor_data
		GROUP BY id
		ORDER BY time DESC
		LIMIT 1
	`

	q := influxdbclient.Query{
		Command:  query,
		Database: config.AppConfig.InfluxDB.Database,
	}

	resp, err := c.client.Query(q)
	if err != nil {
		return nil, err
	}
	if resp.Error() != nil {
		return nil, resp.Error()
	}

	result := make(map[string]*models.SensorData)

	if len(resp.Results) > 0 {
		for _, series := range resp.Results[0].Series {
			if len(series.Values) > 0 {
				sensorID := series.Tags["id"]
				value, _ := series.Values[0][1].(float64)
				setpoint, _ := series.Values[0][2].(float64)
				status, _ := series.Values[0][3].(string)
				ts, _ := time.Parse(time.RFC3339, series.Values[0][0].(string))

				result[sensorID] = &models.SensorData{
					ID:        sensorID,
					Value:     value,
					Setpoint:  setpoint,
					Status:    status,
					Timestamp: ts,
				}
			}
		}
	}

	return result, nil
}

func (c *Client) QueryAggregatedValue(measurement string, field string, stage string, duration time.Duration) (float64, error) {
	var whereClause string
	if stage != "" {
		whereClause = fmt.Sprintf("WHERE stage = '%s' AND time > now() - %dh", stage, int(duration.Hours()))
	} else {
		whereClause = fmt.Sprintf("WHERE time > now() - %dh", int(duration.Hours()))
	}

	query := fmt.Sprintf(`
		SELECT mean(%s) FROM %s
		%s
	`, field, measurement, whereClause)

	q := influxdbclient.Query{
		Command:  query,
		Database: config.AppConfig.InfluxDB.Database,
	}

	resp, err := c.client.Query(q)
	if err != nil {
		return 0, err
	}
	if resp.Error() != nil {
		return 0, resp.Error()
	}

	if len(resp.Results) > 0 && len(resp.Results[0].Series) > 0 && len(resp.Results[0].Series[0].Values) > 0 {
		if val, ok := resp.Results[0].Series[0].Values[0][1].(float64); ok {
			return val, nil
		}
	}

	return 0, fmt.Errorf("no data found")
}

func (c *Client) queryTrend(query string) ([]models.TrendDataPoint, error) {
	q := influxdbclient.Query{
		Command:  query,
		Database: config.AppConfig.InfluxDB.Database,
	}

	resp, err := c.client.Query(q)
	if err != nil {
		return nil, err
	}
	if resp.Error() != nil {
		return nil, resp.Error()
	}

	var points []models.TrendDataPoint

	if len(resp.Results) > 0 && len(resp.Results[0].Series) > 0 {
		for _, row := range resp.Results[0].Series[0].Values {
			ts, err := time.Parse(time.RFC3339, row[0].(string))
			if err != nil {
				continue
			}
			value, _ := row[1].(float64)
			points = append(points, models.TrendDataPoint{
				Timestamp: ts,
				Value:     value,
			})
		}
	}

	return points, nil
}

func (c *Client) CheckSensorOffline(sensorID string, offlineDuration time.Duration) (bool, time.Time, error) {
	query := fmt.Sprintf(`
		SELECT last(value) FROM sensor_data
		WHERE id = '%s'
	`, sensorID)

	q := influxdbclient.Query{
		Command:  query,
		Database: config.AppConfig.InfluxDB.Database,
	}

	resp, err := c.client.Query(q)
	if err != nil {
		return false, time.Time{}, err
	}
	if resp.Error() != nil {
		return false, time.Time{}, resp.Error()
	}

	if len(resp.Results) > 0 && len(resp.Results[0].Series) > 0 && len(resp.Results[0].Series[0].Values) > 0 {
		ts, err := time.Parse(time.RFC3339, resp.Results[0].Series[0].Values[0][0].(string))
		if err != nil {
			return false, time.Time{}, err
		}
		return time.Since(ts) > offlineDuration, ts, nil
	}

	return true, time.Time{}, nil
}
