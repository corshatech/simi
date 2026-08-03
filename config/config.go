/*
Copyright Corsha Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"context"
	"time"
)

// BenchConfig contains benchmarking-specific configuration parameters.
type BenchConfig struct {
	InfluxConfig  `yaml:"influxConfig"`
	OperationType string `yaml:"operationType"`
	ResultSink    struct {
		BrokerURL string `yaml:"brokerUrl" json:"-"`
	} `yaml:"resultSink"`
	OperationPeriod     time.Duration `yaml:"operationPeriod"`
	OperationsPerStream uint32        `yaml:"operationsPerStream"`
	NumWorkers          uint32        `yaml:"numWorkers"`
}

type InfluxConfig struct {
	// the host name and port of the influx server to communicate with
	Url string
	// the bucket to store the metrics in
	Bucket string
}

// OperationConfig represents the custom configuration needed for a chaincode operation
type OperationConfig any

// ChaincodeOperation is a generic function type to run a chaincode operation
type ChaincodeOperation func(context.Context, OperationConfig) error

// ChaincodeOperation is a generic function type to initialize a worker
type InitializeWorker func(string, OperationConfig) (string, error)

// ChaincodeOperation is a generic function type to shutdown a worker
type ShutdownWorker func(OperationConfig)

// WorkerConfig encapsulates the abstracted configuration needed for a benchmarking worker
type WorkerConfig struct {
	OperationConfig OperationConfig `yaml:"operationConfig"`
	WriteOp         ChaincodeOperation
	ReadOp          ChaincodeOperation
	InitFunc        InitializeWorker
	ShutdownFunc    ShutdownWorker
	BenchConfig     `yaml:"benchmark"`
	DataSizeBytes   int
}
