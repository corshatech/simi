/*
Copyright Corsha Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// The consumer command consumes benchmark results over AMQP
package main

import (
	"fmt"
	"log"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
	"gopkg.in/yaml.v2"

	"github.com/corshatech/simi/benchmark/consumer"
	"github.com/corshatech/simi/config"
)

func main() {
	var err error
	defer func() {
		if err != nil {
			log.Fatalln(err)
		}
	}()

	conf, err := configure()
	if err != nil {
		err = fmt.Errorf("failed to load consumer config: %w", err)
		return
	}

	log.Printf("Creating benchmark consumer: %+v\n", conf)

	// Set up AMQP connection
	conn, err := amqp.Dial(conf.ResultSink.BrokerURL)
	if err != nil {
		err = fmt.Errorf("failed to dial AMQP broker at %q: %v", conf.ResultSink.BrokerURL, err)
		return
	}
	defer func() { err = conn.Close() }()

	numRegistered, err := consumer.SyncRegistrations(conn, conf.NumWorkers)
	if err != nil {
		err = fmt.Errorf("failed while starting workers: %w", err)
		return
	}

	conf.NumWorkers = numRegistered
}

// Loads consumer config, returning error if config is invalid.
func configure() (config.BenchConfig, error) {
	const configFileName = "bench-consumer.yaml"
	var conf config.BenchConfig

	yamlFile, err := os.ReadFile(configFileName)
	if err != nil {
		return conf, fmt.Errorf("error reading config file %q: %w", configFileName, err)
	}

	if err = yaml.Unmarshal(yamlFile, &conf); err != nil {
		return conf, fmt.Errorf("error unmarshaling %s: %w", configFileName, err)
	}

	if conf.NumWorkers == 0 {
		return conf, fmt.Errorf("number of workers was zero, nothing to test")
	}

	if conf.OperationsPerStream == 0 {
		return conf, fmt.Errorf("number of operations per stream was zero, nothing to test")
	}

	return conf, nil
}
