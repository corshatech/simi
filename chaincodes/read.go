/*
Copyright Corsha Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincodes

import (
	"context"
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/corshatech/simi/config"
)

func SetupReadOp(c *config.WorkerConfig) error {
	fmt.Println("Increment: Setting up counter op")
	c.ReadOp = func(ctx context.Context, a config.OperationConfig) error {
		worker, ok := a.(*CCWorker)
		if !ok {
			return errors.New("invalid input: expected a CCWorker")
		}
		return worker.CallReadFunc()
	}

	return nil
}

func (a *CCWorker) CallReadFunc() error {

	logger := log.WithFields(log.Fields{
		"funcName": "CCWorker.IncrementKeyAllOrgPeers",
		"org":      org,
	})

	logLevel, err := log.ParseLevel(a.LogLevel)
	if err != nil {
		logger.WithField("logLevel", a.LogLevel).Warn("Unable to parse configured log level. Defaulting to INFO.")
		logLevel = log.DebugLevel
	}
	log.SetLevel(logLevel)

	log.New()

	errIncrement := a.readKey()
	if errIncrement != nil {
		logger.WithError(errIncrement).Errorf("Failed to increment key")
		return fmt.Errorf("failed to increment key: %w", errIncrement)
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
func (a *CCWorker) readKey() error {
	logger := log.WithFields(log.Fields{
		"funcName":  "CCWorker.incrementKey",
		"channelID": channelID,
		"org":       org,
	})

	logger.Warnf("Read key: %s", a.Id)

	peer, err := a.invokeChaincodeCli("query", "experimentRead", [][]byte{[]byte(a.Id)})
	logger = logger.WithField("peer", peer)

	if err == nil {
		logger.Info("Successfully executed experimentRead")
		return nil
	}

	logger.WithError(err).Error("Failed to execute experimentRead function")

	return errors.New("failed to execute experimentRead function")
}
