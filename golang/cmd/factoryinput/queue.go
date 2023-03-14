// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"github.com/beeker1121/goque"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

const queuePath = "/data/factoryinput/queue"

var queue *goque.Queue

func setupQueue() (err error) {
	queue, err = goque.OpenQueue(queuePath)
	if err != nil {
		zap.S().Errorf("Error opening queue: %v", err)
		return
	}
	return
}

func closeQueue() (err error) {
	err = queue.Close()
	if err != nil {
		zap.S().Errorf("Error closing queue: %v", err)
		return
	}
	return
}

func enqueueMQTT(mqttData MQTTData) (err error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	bytes, err := json.Marshal(mqttData)
	if err != nil {
		return
	}
	_, err = queue.Enqueue(bytes)
	if err != nil {
		return err
	}
	return
}

func dequeueMQTT() (mqttData MQTTData, err error) {
	var item *goque.Item
	// Dequeue is internally atomic
	item, err = queue.Dequeue()

	if err != nil {
		return
	}
	if item != nil {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		err = json.Unmarshal(item.Value, &mqttData)
	}
	return
}
