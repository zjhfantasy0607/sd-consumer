package util

import (
	"log"
	"os"
)

var logFiles map[string]*os.File = make(map[string]*os.File)

func Logln(path string, v ...interface{}) {
	// Open the log file in append mode or create it if it doesn't exist
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()

	// Create a new logger that writes to the file
	logger := log.New(file, "", log.LstdFlags)

	// Log the message
	logger.Println(v...)
}

func LogErr(err error, path string) {
	LogFileOn(path)
	log.Printf("\nError: %+v\n\n", err)
	LogFileOff(path)
}

func LogFileOn(logFilePath string) {
	// 创建或打开日志文件
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		LogErr(err, "./log/consumer.log")
		return
	}

	logFiles[logFilePath] = logFile

	// 设置日志输出到文件
	log.SetOutput(logFile)
}

func LogFileOff(logFilePath string) {
	// 关闭文件流
	logFiles[logFilePath].Close()
	// 恢复日志输出到标准输出
	log.SetOutput(os.Stdout)
}
