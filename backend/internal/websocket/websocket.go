package websocket

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	gowebsocket "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"sewage-treatment-system/internal/config"
	"sewage-treatment-system/internal/models"
)

type Client struct {
	conn   *gowebsocket.Conn
	send   chan []byte
	server *Server
	id     string
}

type Server struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	logger     *zap.Logger
	cfg        *config.WebSocketConfig
	upgrader   gowebsocket.Upgrader
}

type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func NewServer(cfg *config.WebSocketConfig, logger *zap.Logger) *Server {
	return &Server{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     logger,
		cfg:        cfg,
		upgrader: gowebsocket.Upgrader{
			ReadBufferSize:  cfg.ReadBufferSize,
			WriteBufferSize: cfg.WriteBufferSize,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *Server) Run() {
	for {
		select {
		case client := <-s.register:
			s.mu.Lock()
			s.clients[client] = true
			s.mu.Unlock()
			s.logger.Info("WebSocket client connected",
				zap.String("client_id", client.id),
				zap.Int("total", len(s.clients)))

		case client := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				close(client.send)
			}
			s.mu.Unlock()
			s.logger.Info("WebSocket client disconnected",
				zap.String("client_id", client.id),
				zap.Int("total", len(s.clients)))

		case message := <-s.broadcast:
			s.mu.RLock()
			for client := range s.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(s.clients, client)
				}
			}
			s.mu.RUnlock()
		}
	}
}

func (s *Server) Broadcast(msgType string, data interface{}) error {
	message := WSMessage{
		Type: msgType,
		Data: data,
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	s.broadcast <- payload
	return nil
}

func (s *Server) BroadcastSensorData(data *models.SensorData) error {
	return s.Broadcast("sensor_data", data)
}

func (s *Server) BroadcastAlarm(alarm *models.Alarm) error {
	return s.Broadcast("alarm", alarm)
}

func (s *Server) BroadcastAerationControl(ctrl *models.AerationControl) error {
	return s.Broadcast("aeration_control", ctrl)
}

func (s *Server) BroadcastCarbonDosing(dosing *models.CarbonDosing) error {
	return s.Broadcast("carbon_dosing", dosing)
}

func (s *Server) BroadcastMetrics(metrics *models.KeyMetrics) error {
	return s.Broadcast("key_metrics", metrics)
}

func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("Failed to upgrade WebSocket", zap.Error(err))
		return
	}

	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		clientID = r.RemoteAddr
	}

	client := &Client{
		conn:   conn,
		send:   make(chan []byte, 256),
		server: s,
		id:     clientID,
	}

	s.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.server.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(time.Duration(c.server.cfg.PongWait) * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(time.Duration(c.server.cfg.PongWait) * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if gowebsocket.IsUnexpectedCloseError(err, gowebsocket.CloseGoingAway, gowebsocket.CloseAbnormalClosure) {
				c.server.logger.Error("WebSocket read error",
					zap.String("client_id", c.id),
					zap.Error(err))
			}
			break
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(time.Duration(c.server.cfg.PingPeriod) * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(time.Duration(c.server.cfg.WriteWait) * time.Second))
			if !ok {
				c.conn.WriteMessage(gowebsocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(gowebsocket.TextMessage, message); err != nil {
				c.server.logger.Error("WebSocket write error",
					zap.String("client_id", c.id),
					zap.Error(err))
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(time.Duration(c.server.cfg.WriteWait) * time.Second))
			if err := c.conn.WriteMessage(gowebsocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *Server) GetClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}
