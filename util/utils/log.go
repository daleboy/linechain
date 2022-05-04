package utils

import (
	"fmt"
	"time"

	"github.com/mattn/go-colorable"
	log "github.com/sirupsen/logrus"
	"github.com/snowzach/rotatefilehook"
)

// SetLog 为每一个实例创建一个log文件，记录日志信息
func SetLog(instanceId string) {
	var logLevel = log.InfoLevel
	filename := "../../logs/console.log"
	if instanceId != "" {
		filename = fmt.Sprintf("../../logs/console_%s.log", instanceId)
	}
	// logrus 的回调钩子
	rotateFileHook, err := rotatefilehook.NewRotateFileHook(rotatefilehook.RotateFileConfig{
		Filename:   filename,
		MaxSize:    50, // 文件最大50M
		MaxBackups: 3,
		MaxAge:     28, //存储28天
		Level:      logLevel,
		Formatter: &log.JSONFormatter{//默认为ASCII formatter，转为JSON formatter
			TimestampFormat: "2006-01-02 15:04:05",//时间戳字符串格式
		},
	})

	if err != nil {
		log.Fatalf("初始化文件回调钩子失败: %v", err)
	}

	log.SetLevel(logLevel)
	log.SetOutput(colorable.NewColorableStdout())
	log.SetFormatter(&log.TextFormatter{
		ForceColors:     true,
		FullTimestamp:   true,
		TimestampFormat: time.RFC822,
	})
	log.AddHook(rotateFileHook)
}
