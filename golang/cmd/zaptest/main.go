package main

import (
	"go.elastic.co/ecszap"
	"go.uber.org/zap"
	"os"
)

func main() {
	defaultECS()
}

func defaultECS() {
	encoderConfig := ecszap.NewDefaultEncoderConfig()
	core := ecszap.NewCore(encoderConfig, os.Stdout, zap.DebugLevel)
	logger := zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	zap.S().Debugf("Hello, %s!", "world")
	zap.S().Infof("Hello, %s!", "world")
	zap.S().Warnf("Hello, %s!", "world")
	zap.S().Errorf("Hello, %s!", "world")
}
