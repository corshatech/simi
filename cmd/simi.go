/*
Copyright Corsha Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	jaegercfg "github.com/uber/jaeger-client-go/config"

	"github.com/corshatech/simi/benchmark/worker"
	"github.com/corshatech/simi/chaincodes"
	"github.com/corshatech/simi/config"
)

func main() {
	numWriters := flag.Int("numWriters", 0, "Number of writers")
	dataSizeStr := flag.String("dataSizeBytes", "", "Size of the data to write")

	flag.Parse()

	log.Printf("Beginning experiment:")

	id, err := strconv.Atoi(os.Getenv("JOB_COMPLETION_INDEX"))
	if err != nil {
		log.Fatalln("Failed to parse ID")
	}

	operationType := "write"
	if id >= *numWriters {
		operationType = "read"
	}

	if err := runSimi(operationType, *numWriters, *dataSizeStr); err != nil {
		log.Fatalln(err)
	}
}

func runSimi(operationType string, numWriters int, dataSizeStr string) error {
	if !opentracing.IsGlobalTracerRegistered() {
		log.Printf("global tracer was NOT initialized on simi boot. building new tracer")
		cfg, jaegerErr := jaegercfg.FromEnv()
		if jaegerErr != nil {
			// parsing errors might happen here, such as when we get a string where we expect a number
			return fmt.Errorf("could not parse Jaeger env vars: %w", jaegerErr)
		}
		cfg.ServiceName = "simi"

		tracer, closer, jaegerErr := cfg.NewTracer()
		if jaegerErr != nil {
			return fmt.Errorf("could not initialize jaeger tracer: %w", jaegerErr)
		}
		defer func() { _ = closer.Close() }()

		opentracing.SetGlobalTracer(tracer)
	} else {
		log.Printf("global tracer was initialized on simi boot. NOT building new tracer")
	}

	var c config.WorkerConfig

	if err := config.LoadConfig(&c); err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	c.OperationType = operationType

	if dataSizeStr != "" {
		if err := config.LoadByteSize(dataSizeStr, &c); err != nil {
			return fmt.Errorf("error loading data size: %w", err)
		}
		log.Printf("Loaded DataSizeBytes %d", c.DataSizeBytes)
	}

	err := setupCustomChaincodes(numWriters, &c)
	if err != nil {
		return fmt.Errorf("error parsing custom Simi configuration: %w", err)
	}

	return benchmarkOperation(c)
}

func setupCustomChaincodes(numWriters int, c *config.WorkerConfig) error {
	err := chaincodes.SetupSimi(c, numWriters)
	if err != nil {
		return err
	}

	err = chaincodes.SetupReadOp(c)
	if err != nil {
		return err
	}

	err = chaincodes.SetupWriteOp(c)
	if err != nil {
		return err
	}

	return nil
}

func benchmarkOperation(c config.WorkerConfig) error {
	log.Info("Running benchmark")

	benchmarkWorker, err := worker.NewWorker(c)
	if err != nil {
		return fmt.Errorf("failed to initialize benchmark worker: %w", err)
	}
	defer benchmarkWorker.Close()

	err = benchmarkWorker.Register()
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}
	err = benchmarkWorker.BenchOperation()
	if err != nil {
		return fmt.Errorf("benchmark writing failed: %w", err)
	}

	return nil
}
