package terminal

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"devplatform/internal/core/platform"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Single-origin platform; CSRF and session cookie protect authenticated endpoints.
		return true
	},
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// RegisterRoutes mounts terminal REST API endpoints on the given router group.
func (s *Service) RegisterRoutes(rg *gin.RouterGroup) {
	terminal := rg.Group("/terminal")
	{
		terminal.GET("/status", s.handleStatus)
		terminal.GET("/servers", s.handleListServers)
		terminal.GET("/servers/:id", s.handleGetServer)
		terminal.POST("/servers", s.handleCreateServer)
		terminal.PUT("/servers/:id", s.handleUpdateServer)
		terminal.DELETE("/servers/:id", s.handleDeleteServer)
		terminal.POST("/servers/test", s.handleTestConnection)
		terminal.GET("/ws/:serverId", s.handleWebSocket)
	}
}

func (s *Service) handleStatus(c *gin.Context) {
	c.JSON(http.StatusOK, s.Status())
}

func (s *Service) handleListServers(c *gin.Context) {
	servers, err := s.ListServers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, servers)
}

func (s *Service) handleGetServer(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server id"})
		return
	}

	server, err := s.GetServer(c.Request.Context(), uint(id))
	if err != nil {
		if errors.Is(err, ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, server)
}

func (s *Service) handleCreateServer(c *gin.Context) {
	var req CreateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := s.CreateServer(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (s *Service) handleUpdateServer(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server id"})
		return
	}

	var req UpdateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := s.UpdateServer(c.Request.Context(), uint(id), req)
	if err != nil {
		if errors.Is(err, ErrLocalImmutable) {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Service) handleDeleteServer(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server id"})
		return
	}

	err = s.DeleteServer(c.Request.Context(), uint(id))
	if err != nil {
		if errors.Is(err, ErrLocalImmutable) {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Service) handleTestConnection(c *gin.Context) {
	var req TestConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.TestConnection(c.Request.Context(), req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "连接测试成功"})
}

func (s *Service) handleWebSocket(c *gin.Context) {
	idParam := c.Param("serverId")
	serverID, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server id"})
		return
	}

	// Read initial cols and rows from query params
	cols := uint16(80)
	rows := uint16(24)
	if qCols := c.Query("cols"); qCols != "" {
		if v, err := strconv.ParseUint(qCols, 10, 16); err == nil && v > 0 {
			cols = uint16(v)
		}
	}
	if qRows := c.Query("rows"); qRows != "" {
		if v, err := strconv.ParseUint(qRows, 10, 16); err == nil && v > 0 {
			rows = uint16(v)
		}
	}

	if serverID == 0 && !s.platform.PTY().Supported() {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "terminal is only supported on Linux"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("terminal ws upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	session, err := s.Connect(c.Request.Context(), uint(serverID), rows, cols)
	if err != nil {
		if errors.Is(err, platform.ErrUnsupported) {
			_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m[错误] 宿主平台不支持本地终端（仅限 Linux）\x1b[0m\r\n"))
		} else {
			_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m[连接失败] "+err.Error()+"\x1b[0m\r\n"))
		}
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()),
			time.Now().Add(time.Second))
		return
	}
	defer func() {
		_ = s.CloseSession(session.ID())
	}()

	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			_ = session.Close()
			_ = conn.Close()
		})
	}

	// Goroutine 1: Session -> WebSocket
	go func() {
		defer closeBoth()
		buf := make([]byte, 4096)
		for {
			n, err := session.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[33m[会话已断开]\x1b[0m\r\n"))
				}
				return
			}
		}
	}()

	// Pump incoming from WebSocket -> Session
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		switch msgType {
		case websocket.BinaryMessage:
			if _, err := session.Write(msg); err != nil {
				return
			}
		case websocket.TextMessage:
			// May be control JSON or plain input
			var ctrl WSClientMessage
			if err := json.Unmarshal(msg, &ctrl); err == nil && ctrl.Type != "" {
				switch ctrl.Type {
				case "resize":
					if ctrl.Cols > 0 && ctrl.Rows > 0 {
						_ = session.Resize(ctrl.Rows, ctrl.Cols)
					}
				case "input":
					if ctrl.Data != "" {
						_, _ = session.Write([]byte(ctrl.Data))
					}
				}
			} else {
				// Raw string input
				_, _ = session.Write(msg)
			}
		}
	}
	closeBoth()
}
