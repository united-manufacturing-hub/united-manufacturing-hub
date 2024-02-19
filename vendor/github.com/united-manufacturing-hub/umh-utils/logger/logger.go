package logger

/*
Copyright 2023 UMH Systems GmbH

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import (
	"errors"
	"os"
	"syscall"

	"go.elastic.co/ecszap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(logLevel string) *zap.SugaredLogger {
	encoderConfig := ecszap.NewDefaultEncoderConfig()
	var core zapcore.Core
	switch logLevel {
	case "DEVELOPMENT":
		core = ecszap.NewCore(encoderConfig, os.Stdout, zap.DebugLevel)
	default:
		core = ecszap.NewCore(encoderConfig, os.Stdout, zap.InfoLevel)
	}
	logger := zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(logger)
	return logger.Sugar()
}

// Wrapper around zap.SugaredLogger.Sync() that ignores EINVAL errors.
//
// See: https://github.com/uber-go/zap/issues/1093#issuecomment-1120667285
func Sync(logger *zap.SugaredLogger) error {
	err := logger.Sync()
	if !errors.Is(err, syscall.EINVAL) {
		return err
	}
	return nil
}
