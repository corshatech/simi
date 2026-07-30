package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"

	"github.com/corshatech/fabric-chaincode/chaincode/internal/chaincode"
	"github.com/corshatech/fabric-chaincode/chaincode/internal/chaincode/metrics"
)

const (
	ccidEnvVarName   = "CHAINCODE_ID"
	ccAddrEnvVarName = "CORE_CHAINCODE_ADDRESS"

	metricsPort            = 11443
	defaultHTTPReadTimeout = 5 * time.Second
	defaultHTTPTimeout     = 30 * time.Second
	shutdownTimeout        = 10 * time.Second
)

func main() {
	// Configure logger to always output timestamp, even when pretty output to TTY
	formatter := &log.TextFormatter{
		FullTimestamp: true,
	}
	log.SetFormatter(formatter)

	// Default logs to INFO level
	log.SetLevel(log.InfoLevel)

	// Set logging level based on env variable
	level := os.Getenv("CORSHA_CHAINCODE_LOGGING_LEVEL")
	if len(level) > 0 {
		parsedLevel, lvlErr := log.ParseLevel(level)
		if lvlErr == nil {
			log.SetLevel(parsedLevel)
		} else {
			log.WithFields(log.Fields{
				"invalid_log_level": level,
				"parse_error":       lvlErr,
			}).Warn("Invalid logging level from env var CORSHA_CHAINCODE_LOGGING_LEVEL, defaulting to INFO")
		}
	}

	logger := log.WithFields(log.Fields{
		"func": "main",
	})

	ccid, found := os.LookupEnv(ccidEnvVarName)
	if !found {
		logger.WithFields(log.Fields{
			"envVar": ccidEnvVarName,
		}).Fatal("Error starting Chaincode - missing REQUIRED ENV VAR")
	}

	addr, found := os.LookupEnv(ccAddrEnvVarName)
	if !found {
		logger.WithFields(log.Fields{
			"envVar": ccAddrEnvVarName,
		}).Fatal("Error starting Chaincode - missing REQUIRED ENV VAR")
	}

	reg := prometheus.NewRegistry()

	mm, err := metrics.NewMetricsManager(reg)
	if err != nil {
		logger.WithError(err).Fatal("Failed to set up metrics manager")
	}

	ccServer := &shim.ChaincodeServer{
		CCID:    ccid,
		Address: addr,
		CC:      chaincode.NewChaincode(mm),
		TLSProps: shim.TLSProperties{
			Disabled: true,
		},
	}

	metricsServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", metricsPort),
		ReadTimeout:  defaultHTTPReadTimeout,
		WriteTimeout: defaultHTTPTimeout,
		IdleTimeout:  defaultHTTPTimeout,
	}

	// Handle Prometheus metrics in a goroutine to avoid blocking other servers.
	// Keep the server shutdown in main() because golang terminates everything immediately, including goroutines and
	// deferred actions, when main() execution completes.
	go func() {
		http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Timeout: defaultHTTPTimeout}))

		// ErrServerClosed is always returned when a server shuts down, so don't kill the program in that case.
		// If we kill the program, we can't gracefully shut down to clean up resources.
		if err := metricsServer.ListenAndServe(); err != nil && errors.Is(err, http.ErrServerClosed) {
			logger.WithError(err).Fatal("Metrics server closed abnormally")
		}
	}()

	logger.WithFields(log.Fields{
		"ccid":    ccid,
		"address": addr,
	}).Info("starting chaincode server")

	// Start the chaincode server
	if err := ccServer.Start(); err != nil {
		logger.WithError(err).Error("Chaincode server closed")
	}

	// Gracefully shut down the metrics server.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.WithError(err).Fatal("Failed to shut down metrics server")
	}
}
