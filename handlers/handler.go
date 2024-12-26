package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"stable-custom/util"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nsqio/go-nsq"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type MessageHandler struct{ Consumer *nsq.Consumer }

var (
	taskId     string
	globalConn *websocket.Conn // 全局 websocket 通道
	connMutex  sync.Mutex
)

func (h *MessageHandler) HandleMessage(m *nsq.Message) error {
	var err error

	// 获取加密key
	key := viper.GetString("appkey")

	// 读取消息队列中的 json 字段
	msgJson := string(m.Body)
	gTaskId := gjson.Get(msgJson, "task_id")
	gApi := gjson.Get(msgJson, "api")
	gParams := gjson.Get(msgJson, "params")

	// 加密 taskId
	taskId, err = util.Encrypt(gTaskId.String(), key)
	if err != nil {
		err = fmt.Errorf("taskid encrypt error: %w", err)
		util.LogErr(errors.WithStack(err), "./log/consumer.log")
		return nil
	}

	// 根据handle字段调用相对应的handler生成图片
	err = callSdApi(gApi.String(), gParams.String())
	if err != nil {
		util.LogErr(err, "./log/consumer.log")
	}

	return nil
}

func createGlobalConn() {
	// 检查当前 socket 通道是否可用，不可用时创建新的 socket 通道
	if globalConn == nil {
		wshost := viper.GetString("MainServer.ws")
		port := viper.GetString("MainServer.port")
		url := wshost + ":" + port + "/sd-callback"

		conn, err := NewWSClient(url)
		if err != nil {
			util.LogErr(err, "./log/socket.log")
		}

		conn.SetReadDeadline(time.Now().Add(ReadTimeout))
		conn.SetPingHandler(func(appData string) error {
			connMutex.Lock()
			defer connMutex.Unlock()

			conn.SetReadDeadline(time.Now().Add(ReadTimeout))
			// 发送 pong 消息
			err := conn.WriteMessage(websocket.PongMessage, []byte(appData))
			if err != nil {
				err = fmt.Errorf("failed to send pong: %v", err)
				closeGlobalConn()
				return err
			}

			return nil
		})

		// 设置关闭处理器
		conn.SetCloseHandler(func(code int, text string) error {
			connMutex.Lock()
			defer connMutex.Unlock()

			// 回复关闭消息
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			closeGlobalConn()
			return nil // 返回错误会被 ReadMessage 捕获
		})

		globalConn = conn
	}

	// 读取一次消息，启动自动控制帧操作，此业务场景无需接受消息，故只读一次(收到 ping 时会返回 pong)
	go func() {
		_, _, err := globalConn.ReadMessage()
		if err != nil {
			closeGlobalConn()
			return
		}
	}()
}

func closeGlobalConn() {
	if globalConn != nil {
		globalConn.Close() // 确保关闭连接释放资源
		globalConn = nil
	}
}

// 使用 socket 通道传递 sdapi 的响应内容
func writeJson(api string, status int, body string) error {
	connMutex.Lock()
	defer connMutex.Unlock()

	if globalConn == nil {
		createGlobalConn()
	}

	json, _ := sjson.Set("{}", "api", api)
	json, _ = sjson.Set(json, "task_id", taskId)
	json, _ = sjson.Set(json, "status", status)
	json, _ = sjson.Set(json, "body", body)

	if err := globalConn.WriteJSON(json); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func callSdApi(api string, params string) error {
	// 创建一个 context，用于控制 progress 的停止
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动 progress 的 Goroutine
	go func() {
		ticker := time.NewTicker(1000 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return // 停止 progress 的执行
			case <-ticker.C:
				if err := progress(); err != nil {
					util.LogErr(errors.WithStack(err), "./log/consumer.log")
				}
			}
		}
	}()

	// 使用默认客户端发送请求
	client := &http.Client{}

	// 创建新的POST请求
	url := ApiHost(api)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(params)))
	if err != nil {
		return errors.WithStack(err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 使用HTTP客户端发送请求
	resp, err := client.Do(req)
	if err != nil {
		// socket 错误场景返回错误
		writeJson(api, 509, string("stable diffusion server error"))
		return errors.WithStack(err)
	}
	defer resp.Body.Close()

	// 读取响应体
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.WithStack(err)
	}

	// socket 转发请求到的信息
	writeJson(api, resp.StatusCode, string(bodyBytes))

	return nil
}

func progress() error {
	url := ApiHost("sdapi/v1/progress")
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 读取响应体
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.WithStack(err)
	}

	gProgress := gjson.Get(string(bodyBytes), "progress")

	// socket 发送信息
	writeJson("sdapi/v1/progress", resp.StatusCode, gProgress.String())

	return nil
}

// 拼接 stable diffusion api 地址
func ApiHost(api string) string {
	// 过滤字符串开头的自行拼接 "/"
	api = strings.TrimPrefix(api, "/")

	// 获取配置值
	host := viper.GetString("StableDiffusion.host")
	port := viper.GetString("StableDiffusion.port")

	return host + ":" + port + "/" + api
}

// 拼接主服务器地址
func MainHost(uri string) string {
	// 过滤字符串开头的自行拼接 "/"
	uri = strings.TrimPrefix(uri, "/")

	// 获取配置值
	host := viper.GetString("MainServer.host")
	port := viper.GetString("MainServer.port")

	return host + ":" + port + "/" + uri
}
