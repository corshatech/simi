/*
Copyright Corsha Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincodes

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/corshatech/simi/config"
)

func SetupWriteOp(c *config.WorkerConfig) error {
	c.WriteOp = func(ctx context.Context, a config.OperationConfig) error {
		worker, ok := a.(*CCWorker)
		if !ok {
			return errors.New("invalid input: expected a CCWorker")
		}
		return worker.CallWriteFunc(c.DataSizeBytes)
	}

	return nil
}

func (a *CCWorker) CallWriteFunc(dataSizeBytes int) error {
	logger := log.WithFields(log.Fields{
		"funcName": "write.CallWriteFunc",
		"org":      org,
	})

	logLevel, err := log.ParseLevel(a.LogLevel)
	if err != nil {
		logger.WithField("logLevel", a.LogLevel).Warn("Unable to parse configured log level. Defaulting to INFO.")
		logLevel = log.DebugLevel
	}
	log.SetLevel(logLevel)

	log.New()

	errIncrement := a.writeKey(dataSizeBytes)
	if errIncrement != nil {
		logger.WithError(errIncrement).Errorf("Failed to write against peer")
		return fmt.Errorf("failed to write key: %w", errIncrement)
	}

	return nil
}

// incrementKey executes the core chaincode operation on a specific peer to increment the
// counter value. This method handles the low-level details of chaincode invocation,
// including channel client creation, peer targeting, and retry logic for network resilience.
// The function implements a robust retry mechanism to handle temporary network issues,
// chaincode deployment delays, and other transient failures that are common in distributed
// blockchain networks. The chaincode function being invoked will check if a counter key
// exists for the given peer identifier, initialize it to 0 if not present, or increment
// the existing value by 1, implementing the core business logic of the counter application.
func (a *CCWorker) writeKey(dataSizeBytes int) error {
	logger := log.WithFields(log.Fields{
		"funcName":  "CCWorker.writeKey",
		"channelID": channelID,
		"org":       org,
	})

	logger.Info("Writing with timeout...")

	// Generates a string of the given size; ASCII characters are each 1 byte
	data := strings.Repeat("a", dataSizeBytes)

	logger.Warnf("Write key: %s", a.Id)

	peer, err := a.invokeChaincodeCli("invoke", "experimentWrite", [][]byte{[]byte(a.Id), []byte(strconv.Itoa(a.Counter)), []byte(data)})
	logger = logger.WithField("peer", peer)

	if err == nil {
		logger.Info("Successfully executed experimentWrite")
		a.Counter += 1
		return nil
	}

	logger.WithError(err).Error("Failed to execute experimentWrite function")

	return errors.New("failed to execute experimentWrite function")
}
