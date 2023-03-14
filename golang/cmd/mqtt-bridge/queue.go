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
	"go.uber.org/zap"
)

type queueObject struct {
	Topic   string
	Message []byte
}

const queuePath = "/data/queue"

func setupQueue(mode string) (pq *goque.Queue, err error) {
	pq, err = goque.OpenQueue(queuePath + "/" + mode)
	if err != nil {
		zap.S().Errorf("Error opening queue", err)
		return
	}
	return
}

func closeQueue(pq *goque.Queue) (err error) {

	err = pq.Close()

	if err != nil {
		zap.S().Errorf("Error closing queue", err)
		return
	}

	return
}

func storeMessageIntoQueue(topic string, message []byte, pq *goque.Queue) {

	newElement := queueObject{
		Topic:   topic,
		Message: message,
	}

	_, err := pq.EnqueueObject(newElement)
	if err != nil {
		zap.S().Errorf("Error enqueuing", err)
		return
	}
}
