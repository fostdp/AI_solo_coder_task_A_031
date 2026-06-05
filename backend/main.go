package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"sewage-treatment/backend/alarm"
	"sewage-treatment/backend/api"
	"sewage-treatment/backend/config"
	"sewage-treatment/backend/control"
	"sewage-treatment/backend/influxdb"
	"sewage-treatment/backend/mqtt"
	"sewage-treatment/backend/models"
	ws "sewage-treatment/backend/websocket"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Sewage Treatment Control System...")

	config.LoadConfig()
	models.InitSensorConfigs()
	log.Printf("Loaded %d sensor configurations", len(models.SensorConfigs))

	if err := influxdb.NewClient(); err != nil {
		log.Fatalf("Failed to connect to InfluxDB: %v", err)
	}
	defer influxdb.InfluxClient.Close()

	ws.WSHub = ws.NewHub()
	go ws.WSHub.Run()

	alarm.AlarmMgr = alarm.NewAlarmManager(ws.PushAlarm)
	alarm.AlarmMgr.Start()
	defer alarm.AlarmMgr.Stop()

	if err := mqtt.NewClient(); err != nil {
		log.Printf("Warning: Failed to connect to MQTT broker: %v", err)
		log.Println("Continuing without MQTT connection...")
	}
	defer mqtt.Disconnect()

	go forwardSensorDataToWebSocket()

	control.AerationCtl = control.NewAerationController()
	control.AerationCtl.Start()
	defer control.AerationCtl.Stop()

	control.CarbonOpt = control.NewCarbonOptimizer()
	control.CarbonOpt.Start()
	defer control.CarbonOpt.Stop()

	control.StartKPICalculation()
	ws.StartPeriodicPush()
	go periodicControlPush()

	r := gin.Default()

	r.Use(CORS())

	r.Static("/static", "./frontend")
	r.GET("/", func(c *gin.Context) {
		c.File("./frontend/index.html")
	})

	r.GET("/ws", ws.HandleWebSocket)

	api.SetupRoutes(r)

	srv := &http.Server{
		Addr:    ":" + config.AppConfig.Server.Port,
		Handler: r,
	}

	go func() {
		log.Printf("HTTP server starting on port %s", config.AppConfig.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func forwardSensorDataToWebSocket() {
	sensorChan := mqtt.GetSensorDataChannel()
	for data := range sensorChan {
		ws.PushSensorUpdate(data)
		alarm.BroadcastSensorUpdate(data)
	}
}

func periodicControlPush() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		if control.AerationCtl != nil && control.CarbonOpt != nil {
			aerationStatus := control.AerationCtl.GetAllStatus()
			carbonStatus := control.CarbonOpt.GetStatus()
			ws.PushControlUpdate(aerationStatus, carbonStatus)

			energy, carbon, removal := control.CalculateKPIValues()
			ws.PushKPIUpdate(energy, carbon, removal)
		}
	}
}
