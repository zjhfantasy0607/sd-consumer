package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"stable-custom/handlers"
	"syscall"
	"time"

	"github.com/nsqio/go-nsq"
	"github.com/spf13/viper"
)

func main() {
	// 初始化配置文件
	InitConfig()

	config := nsq.NewConfig()
	// config.MaxAttempts = 11 // 设置最大失败重新尝试次数为11次
	config.MsgTimeout = time.Second * 60 * 15 // 15分钟的超时时间

	topic := viper.GetString("NSQLookupd.topic")
	channel := viper.GetString("NSQLookupd.channelName")
	consumer, err := nsq.NewConsumer(topic, channel, config)
	if err != nil {
		log.Fatalf("could not create consumer: %v", err)
	}

	consumer.AddHandler(&handlers.MessageHandler{Consumer: consumer})

	// 读取NSQLOOKUPD配置
	host := viper.GetString("NSQLookupd.host")
	port := viper.GetString("NSQLookupd.port")
	nsqlookupdAddress := fmt.Sprintf("%s:%s", host, port)

	err = consumer.ConnectToNSQLookupd(nsqlookupdAddress)
	if err != nil {
		log.Fatalf("could not connect to nsqlookupd: %v", err)
	}

	// wait for signal to exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Gracefully stop the consumer.
	consumer.Stop()
}

func InitConfig() {
	// 获取环境变量的值
	env := os.Getenv("JUGG_TOOL_BOX_SERVICE_ENV")

	// 根据当前环境读取不同的配置文件
	fileName := "application"
	if env == "product" {
		fileName = fileName + ".product"
	}

	workDir, _ := os.Getwd()
	viper.SetConfigName(fileName)
	viper.SetConfigType("yml")
	viper.AddConfigPath(workDir + "/config")
	err := viper.ReadInConfig()

	if err != nil {
		panic("config file load fail")
	}
}
