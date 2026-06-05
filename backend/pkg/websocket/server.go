package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"sewage-plant-system/pkg/models"
)

type Server struct {
	upgrader      websocket.Upgrader
	clients       map[*websocket.Conn]bool
	broadcast     chan interface{}
	mu            sync.RWMutex
	alertChannel  chan *models.Alert
	dataChannel   chan interface{}
	statusChange  func(connected bool, clientCount int)
	lastStatus    bool
}

type Message struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

func NewServer() *Server {
	return &Server{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		clients:      make(map[*websocket.Conn]bool),
		broadcast:    make(chan interface{}, 100),
		alertChannel: make(chan *models.Alert, 100),
		dataChannel:  make(chan interface{}, 100),
		lastStatus:   false,
	}
}

func (s *Server) SetStatusChangeCallback(callback func(connected bool, clientCount int)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusChange = callback
	log.Printf("[WS] Status change callback registered")
}

func (s *Server) notifyStatusChange() {
	s.mu.RLock()
	callback := s.statusChange
	clientCount := len(s.clients)
	connected := clientCount > 0
	statusChanged := connected != s.lastStatus
	s.mu.RUnlock()

	if statusChanged {
		s.mu.Lock()
		s.lastStatus = connected
		s.mu.Unlock()

		if callback != nil {
			log.Printf("[WS] Status changed: connected=%v, clients=%d, notifying callback", connected, clientCount)
			callback(connected, clientCount)
		}
	}
}

func (s *Server) HasClients() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients) > 0
}

func (s *Server) HandleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	s.mu.Lock()
	s.clients[conn] = true
	s.mu.Unlock()

	log.Printf("WebSocket client connected: %s", conn.RemoteAddr())

	go s.notifyStatusChange()

	go s.handleClient(conn)

	s.sendInitialData(conn)
}

func (s *Server) handleClient(conn *websocket.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
		log.Printf("WebSocket client disconnected: %s", conn.RemoteAddr())
		go s.notifyStatusChange()
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}
	}
}

func (s *Server) sendInitialData(conn *websocket.Conn) {
	msg := Message{
		Type:      "welcome",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"message": "Connected to Sewage Plant Monitoring System",
			"time":    time.Now(),
		},
	}
	s.sendMessage(conn, msg)
}

func (s *Server) sendMessage(conn *websocket.Conn, msg Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket marshal error: %v", err)
		return
	}

	err = conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		log.Printf("WebSocket write error: %v", err)
	}
}

func (s *Server) Broadcast(msgType string, data interface{}) {
	msg := Message{
		Type:      msgType,
		Timestamp: time.Now(),
		Data:      data,
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for conn := range s.clients {
		go s.sendMessage(conn, msg)
	}
}

func (s *Server) BroadcastAlert(alert *models.Alert) error {
	return s.BroadcastWithResult("alert", alert)
}

func (s *Server) BroadcastWithResult(msgType string, data interface{}) error {
	msg := Message{
		Type:      msgType,
		Timestamp: time.Now(),
		Data:      data,
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.clients) == 0 {
		return fmt.Errorf("no WebSocket clients connected")
	}

	dataBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	var sendErrors []error
	for conn := range s.clients {
		err := conn.WriteMessage(websocket.TextMessage, dataBytes)
		if err != nil {
			sendErrors = append(sendErrors, fmt.Errorf("client %s: %w", conn.RemoteAddr(), err))
		}
	}

	if len(sendErrors) > 0 {
		return fmt.Errorf("partial broadcast failure: %d/%d clients failed: %v",
			len(sendErrors), len(s.clients), sendErrors[0])
	}

	return nil
}

func (s *Server) BroadcastSensorData(data *models.SensorData) {
	s.Broadcast("sensor_data", data)
}

func (s *Server) BroadcastPLCStatus(status *models.PLCStatus) {
	s.Broadcast("plc_status", status)
}

func (s *Server) BroadcastControlCommand(cmd *models.ControlCommand) {
	s.Broadcast("control_command", cmd)
}

func (s *Server) BroadcastAerationControl(data map[string]interface{}) {
	s.Broadcast("aeration_control", data)
}

func (s *Server) BroadcastCarbonControl(data map[string]interface{}) {
	s.Broadcast("carbon_control", data)
}

func (s *Server) BroadcastKPI(kpi *models.KPIData) {
	s.Broadcast("kpi", kpi)
}

func (s *Server) BroadcastSystemStatus(status map[string]interface{}) {
	s.Broadcast("system_status", status)
}

func (s *Server) GetClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

func (s *Server) Start() {
	log.Println("WebSocket server started")
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for conn := range s.clients {
		conn.Close()
		delete(s.clients, conn)
	}

	close(s.broadcast)
	close(s.alertChannel)
	close(s.dataChannel)
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
