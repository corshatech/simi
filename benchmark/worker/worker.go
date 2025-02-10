/*
Copyright Corsha Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/opentracing/opentracing-go"
	amqp "github.com/rabbitmq/amqp091-go"
	log "github.com/sirupsen/logrus"

	"github.com/corshatech/simi/benchmark"
	"github.com/corshatech/simi/config"

	_ "github.com/lib/pq"
)

const (
	DefaultJobID = "simi"
	simiNameEnv  = "SIMI_NAME"
	influxTokEnv = "INFLUX_TOKEN"

	// startTimeout is the maximum time a simi will wait for a "start" command
	// after the simi has sent its "registration complete" message
	startTimeout = 10 * time.Minute
)

type Worker struct {
	influxClient  influxdb2.Client
	writeAPI      api.WriteAPI
	conn          *amqp.Connection
	correlationID string
	username      string
	config        config.WorkerConfig
}

// NewWorker connects to the AMQP broker and starts the benchmark reporter.
func NewWorker(conf config.WorkerConfig) (*Worker, error) {
	conn, err := amqp.Dial(conf.BenchConfig.ResultSink.BrokerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to dial AMQP broker at %q: %w", conf.BenchConfig.ResultSink.BrokerURL, err)
	}

	influxTok, found := os.LookupEnv(influxTokEnv)
	if !found {
		return nil, errors.New("could not find required variable: " + influxTokEnv)
	}

	influxClient := influxdb2.NewClient(conf.BenchConfig.InfluxConfig.Url, influxTok)
	// Get non-blocking write client
	writeAPI := influxClient.WriteAPI("simi", conf.BenchConfig.InfluxConfig.Bucket)

	// handle async write errors
	errorsCh := writeAPI.Errors()
	go func() {
		for err := range errorsCh {
			log.WithError(err).Error("Influx write error")
		}
	}()

	correlationID := os.Getenv(simiNameEnv)

	return &Worker{
		config:        conf,
		conn:          conn,
		correlationID: correlationID,
		influxClient:  influxClient,
		writeAPI:      writeAPI,
	}, nil
}

// Close closes the connection used by this worker.
func (w *Worker) Close() {
	if w.config.ShutdownFunc != nil {
		w.config.ShutdownFunc(w.config.OperationConfig)
	}
	if w.conn != nil {
		_ = w.conn.Close()
	}
	if w.writeAPI != nil {
		w.writeAPI.Flush()
	}
	if w.influxClient != nil {
		w.influxClient.Close()
	}
}

// Register performs registration for the requested number of workers.
// Upon successful registration the Worker publishes its success status to the Consumer.
// The Worker then waits for a Start signal from the Consumer before beginning its benchmarking tasks.
func (w *Worker) Register() error {
	// Channel for publishing registration status and receiving start command
	ch, err := w.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to create registration notification channel: %w", err)
	}
	defer ch.Close()

	// Command consumer queue
	startCommandQueue, err := ch.QueueDeclare(
		"",
		false,
		true,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	// Start command is received via topic exchange
	err = ch.QueueBind(
		startCommandQueue.Name,
		"start",
		"amq.topic",
		false,
		nil,
	)
	if err != nil {
		return err
	}

	// Begin consuming start commands
	startCommands, err := ch.Consume(
		startCommandQueue.Name,
		"",
		true,
		true,
		false,
		false,
		nil)
	if err != nil {
		return err
	}

	var registrationErr error
	w.username, registrationErr = w.config.InitFunc(w.correlationID, w.config.OperationConfig)

	// Registration publication queue
	regQueue, err := ch.QueueDeclare(
		"registrations",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare registration notification queue: %w", err)
	}

	// errorText indicates success with an empty string
	var errorText []byte
	var regErrored bool
	if registrationErr != nil {
		errorText = []byte(registrationErr.Error())
		regErrored = true
	}

	// Publish this registration status
	err = ch.Publish(
		"",
		regQueue.Name,
		true,
		false,
		amqp.Publishing{
			ContentType:   "text/plain",
			Body:          errorText,
			CorrelationId: w.username + w.correlationID,
			Timestamp:     time.Now(),
		})
	if err != nil {
		return fmt.Errorf("failed to publish benchmark result: %w", err)
	}

	if regErrored {
		return fmt.Errorf("failed to register: %w", registrationErr)
	}

	log.Println("Registration complete, waiting for start command")
	timer := time.NewTimer(startTimeout)
	for {
		select {
		case <-startCommands:
			log.Info("Received start command, starting benchmark operations")
			return nil
		case <-timer.C:
			log.Warn("Timed out waiting for start command, starting benchmark operations")
			return nil
		}
	}
}

// BenchOperation benchmarks the operation being exercised.
//
//nolint:gocyclo
func (w *Worker) BenchOperation() error {

	log.Printf("Benchmarking %s for %d iterations per stream", w.config.OperationType, w.config.OperationsPerStream)

	result := &benchmark.Result{
		Min: benchmark.MaxDuration.Nanoseconds(),
	}
	workerStart := time.Now()
	result.Start = workerStart.UnixNano()

	operationName := w.config.BenchConfig.OperationType
	benchFunc := func() error {
		span, ctx := opentracing.StartSpanFromContext(context.Background(), operationName)
		if span == nil {
			return fmt.Errorf("error starting %s span", operationName)
		}
		defer span.Finish()
		return w.config.ChaincodeOp(ctx, w.config.OperationConfig)
	}

	for i := uint32(0); i < w.config.OperationsPerStream; i++ {
		// clear previous state
		result.Err = ""

		startTime := time.Now()

		// Execute the function being benchmarked
		err := benchFunc()
		if err != nil {
			result.Err = err.Error()
		} else {
			elapsed := time.Since(startTime).Nanoseconds()
			if elapsed < result.Min {
				result.Min = elapsed
			}

			if elapsed > result.Max {
				result.Max = elapsed
			}
			// push duration to Influx
			p := influxdb2.NewPointWithMeasurement(w.config.OperationType).
				AddField("duration", elapsed).
				SetTime(startTime)
			w.writeAPI.WritePoint(p)

			result.AvgTotal += elapsed

			result.Elapsed = time.Since(workerStart).Nanoseconds()
		}

		time.Sleep(w.config.OperationPeriod)
	}

	log.WithFields(log.Fields{
		"result": fmt.Sprintf("%+v", result),
	}).Printf("Benchmark finished")

	return nil
}
