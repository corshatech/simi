/*
Copyright Corsha Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"

	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	jaegercfg "github.com/uber/jaeger-client-go/config"

	"github.com/corshatech/simi/benchmark/worker"
	"github.com/corshatech/simi/config"
	"github.com/corshatech/simi/ping"
)

func main() {
	if err := runSimi(); err != nil {
		log.Fatalln(err)
	}
}

func runSimi() error {
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

	err := setupCustomSimi(&c)
	if err != nil {
		return fmt.Errorf("error parsing custom Simi configuration: %w", err)
	}

	return benchmarkOperation(c)
}

func setupCustomSimi(c *config.WorkerConfig) error {
	return ping.SetupPingSimi(c)
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
