package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

const (
	ReadTimeout = 60 * time.Second // 连接读取超时
)

func NewWSClient(url string) (*websocket.Conn, error) {
	// 发起 WebSocket 连接
	conn, _, err := websocket.DefaultDialer.Dial(url, http.Header{})
	if err != nil {
		return nil, errors.WithStack(fmt.Errorf("连接 WebSocket 服务端失败: %w", err))
	}

	conn.SetReadDeadline(time.Now().Add(ReadTimeout))
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(ReadTimeout))
		// 发送 pong 消息
		err := conn.WriteMessage(websocket.PongMessage, []byte(appData))
		if err != nil {
			err = fmt.Errorf("failed to send pong: %v", err)
			return err
		}
		return nil
	})

	return conn, nil
}
