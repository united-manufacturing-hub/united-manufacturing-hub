package main

import (
	"context"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net"
	"strings"
	"time"
)

func resolver(brokers []string) []string {
	zap.S().Debugf("Resolving brokers: %v", brokers)
	var newBrokers []string
	for _, broker := range brokers {
		splits := strings.Split(broker, ":")
		if len(splits) != 2 {
			zap.S().Warnf("Appending default port (9092) to broker: %s", broker)
			splits = append(splits, "9092")
		}
		name := splits[0]
		port := splits[1]

		ip := net.ParseIP(name)

		if ip != nil {
			zap.S().Debugf("Broker %s is an IP address", broker)
			newBrokers = append(newBrokers, broker)
		} else {
			nameX := resolveDNS(name)
			zap.S().Debugf("Broker %s resolved to %s", name, nameX)
			newBrokers = append(newBrokers, nameX+":"+port)
		}
	}
	return newBrokers
}

func resolveDNS(name string) string {
	_, err := net.LookupHost(name)
	if err == nil {
		return name
	}
	// This might be a kubernetes pod name
	// use kubectl get pod united-manufacturing-hub-kafka-0 -n united-manufacturing-hub -o=jsonpath='{.status.podIP}'

	config, err := rest.InClusterConfig()
	if err != nil {
		zap.S().Fatal(err)
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		zap.S().Fatal(err)
	}

	ctx, cncl := context.WithTimeout(context.Background(), 5*time.Second)
	defer cncl()

	pod, err := clientset.CoreV1().Pods("united-manufacturing-hub").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		zap.S().Fatal(err)
	}
	// Check if the pod is running
	if pod.Status.Phase != "Running" {
		zap.S().Fatal("Pod is not running")
	}

	return pod.Status.PodIP
}
