package main

import (
	"flag"
	"strings"
	"time"

	"github.com/go-redis/redis"
	log "github.com/sirupsen/logrus"
)

var r *redis.Client

func main() {
	// Command line flags
	logDebug := flag.Bool("debug", false, "Set log level to debug")
	logTrace := flag.Bool("trace", false, "Set log level to trace")

	flag.Parse()

	if *logTrace {
		log.Info("Setting log level to trace")
		log.SetLevel(log.TraceLevel)
	} else if *logDebug {
		log.Info("Setting log level to debug")
		log.SetLevel(log.DebugLevel)
	}

	r = redis.NewClient(&redis.Options{
		Network: "unix",
		Addr:    "/var/run/redis/redis.sock",
	})

	_, err := r.Ping().Result()
	if err != nil {
		panic(err)
	}

	pubsub := r.PSubscribe("__keyspace@0__:/redfish/v1/Chassis/*/Actions/Chassis.Reset/Target")

	ch := pubsub.Channel()

	for msg := range ch {
		log.Info("Received message")
		log.Info("Channel: ", msg.Channel)
		log.Info("Pattern: ", msg.Pattern)
		log.Info("Payload: ", msg.Payload)

		if msg.Payload != "rename_to" {
			log.Info("Ingnoring non-rename_to message")
			continue
		}

		if !strings.HasPrefix(msg.Channel, "__keyspace@0__:") {
			log.Warn("Invalid channel")
			continue
		}
		key := strings.TrimPrefix(msg.Channel, "__keyspace@0__:")
		go handleIPC(key)

	}
}

// Handle IPC request
func handleIPC(targetKey string) {
	// Retrieve request
	request, err := r.HGetAll(targetKey).Result()
	if err != nil {
		panic(err)
	}

	responseKey := request["ResponseKey"]
	resetType := request["ResetType"]
	log.Info("ResetType: ", resetType)

	// Process request
	result := "Bad Key"
	if targetKey == "/redfish/v1/Chassis/APC/Actions/Chassis.Reset/Target" {
		log.Info("Performing work...")
		time.Sleep(2 * time.Second)
		result = "Success"
	}

	response := map[string]interface{}{
		"Result": result,
	}
	// Set response key to notify the frontend that the actions is done
	log.Info("Setting response")
	err = r.HMSet(responseKey, response).Err()
	if err != nil {
		panic(err)
	}

	// Delete target key
	log.Debug("Deleting key: ", targetKey)
	err = r.Del(targetKey).Err()
	if err != nil {
		panic(err)
	}
}
