package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"sewage-treatment/backend/models"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

var WSHub *Hub

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte, 1000),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("WebSocket client connected, total clients: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("WebSocket client disconnected, total clients: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) Broadcast(message []byte) {
	select {
	case h.broadcast <- message:
	default:
		log.Printf("WebSocket broadcast channel full, dropping message")
	}
}

func (c *Client) readPump() {
	defer func() {
		WSHub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket: %v", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
	}

	WSHub.register <- client

	go client.writePump()
	go client.readPump()
}

func PushAlarm(alarm *models.Alarm) {
	if WSHub == nil {
		return
	}

	msg := map[string]interface{}{
		"type":      "alarm",
		"alarm":    alarm,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal alarm for WebSocket: %v", err)
		return
	}

	WSHub.Broadcast(data)
}

func PushSensorUpdate(data *models.SensorData) {
	if WSHub == nil {
		return
	}

	status := calculateSensorStatus(data)

	msg := map[string]interface{}{
		"type":   "sensor_update",
		"sensor": status,
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal sensor update: %v", err)
		return
	}

	WSHub.Broadcast(jsonData)
}

func calculateSensorStatus(data *models.SensorData) *models.SensorStatus {
	cfg := models.GetSensorConfig(data.ID)
	if cfg == nil {
		return &models.SensorStatus{
			ID:         data.ID,
			Type:       string(data.Type),
			Stage:      string(data.Stage),
			Value:      data.Value,
			Deviation:  0,
			Color:      "#4CAF50",
			LastUpdate: data.Timestamp,
			Online:     true,
		}
	}

	target := (cfg.TargetMin + cfg.TargetMax) / 2
	rangeVal := cfg.TargetMax - cfg.TargetMin
	if rangeVal == 0 {
		rangeVal = 1
	}

	deviation := 0.0
	if data.Value > target {
		deviation = (data.Value - target) / rangeVal
	} else {
		deviation = (target - data.Value) / rangeVal
	}

	deviationPercent := deviation * 100

	color := "#4CAF50"
	if deviationPercent > 20 {
		color = "#f44336"
	} else if deviationPercent > 10 {
		color = "#ff9800"
	}

	return &models.SensorStatus{
		ID:         data.ID,
		Type:       string(data.Type),
		Stage:      string(data.Stage),
		Value:      data.Value,
		Deviation:  deviationPercent,
		Color:      color,
		LastUpdate: data.Timestamp,
		Online:     true,
	}
}

func PushKPIUpdate(energy, carbon, removal float64) {
	if WSHub == nil {
		return
	}

	msg := map[string]interface{}{
		"type": "kpi_update",
		"kpi": map[string]float64{
			"energy":   energy,
			"carbon": carbon,
			"removal": removal,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal KPI for WebSocket: %v", err)
		return
	}

	WSHub.Broadcast(data)
}

func PushControlUpdate(aerationStatus []*models.AerationControl, carbonStatus *models.CarbonControl) {
	if WSHub == nil {
		return
	}

	msg := map[string]interface{}{
		"type":    "control_update",
		"aeration": aerationStatus,
		"carbon":  carbonStatus,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal control update: %v", err)
		return
	}

	WSHub.Broadcast(data)
}

func StartPeriodicPush() {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			PushSystemStatus()
		}
	}()
	log.Println("Periodic WebSocket push started")
}

func PushSystemStatus() {
	if WSHub == nil {
		return
	}

	msg := map[string]interface{}{
		"type": "heartbeat",
		"timestamp": time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal heartbeat: %v", err)
		return
	}

	WSHub.Broadcast(data)
}
