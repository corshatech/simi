/*
Copyright Corsha Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ping

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/channel"
	contextAPI "github.com/hyperledger/fabric-sdk-go/pkg/common/providers/context"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/fab"
	fabricConfig "github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
	"github.com/iancoleman/strcase"
	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"

	"github.com/corshatech/simi/config"
)

type PingWorker struct {
	ChaincodeID string `yaml:"chaincodeID"`
	Sdk         *fabsdk.FabricSDK
	Username    string `yaml:"username"`
	Org         string `yaml:"org"`
	LogLevel    string `yaml:"logLevel"`
}

const (
	channelID     = "benchmarkchannel"
	sdkConfigPath = "fabric-config.yaml"
)

func SetupPingSimi(c *config.WorkerConfig) error {
	logger := log.WithFields(log.Fields{
		"funcName": "admin.SetupPingSimi",
	})
	var pingWorker *PingWorker
	configMetadata := mapstructure.Metadata{}

	// Decode raw interface into ProxyConfiguration struct and metadata
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,            // allows weak conversions described here: https://pkg.go.dev/github.com/mitchellh/mapstructure@v1.5.0#Decoder.Decode
		Result:           &pingWorker,     // PingWorker struct
		Metadata:         &configMetadata, // tracks used, unused, and unset keys
		TagName:          "yaml",          // fieldName in MatchName function below will be derived from the protobuf json tag
		MatchName: func(mapKey, fieldName string) bool { // convert mapKey (yaml camelcase name) to snake case and match the fieldName (from protobuf json tag)
			return strcase.ToSnake(mapKey) == fieldName
		},
	})
	if err != nil {
		logger.Errorf("Failed to create decoder: %v", err)
		return err
	}
	err = decoder.Decode(c.OperationConfig)
	if err != nil {
		logger.Errorf("Failed to decode pingWorker: %v", err)
		logger.Debugf("Metadata.Unused: %#v", configMetadata.Unused)
		logger.Debugf("Metadata.Unset: %#v", configMetadata.Unset)
		return err
	}
	c.OperationConfig = pingWorker
	c.ChaincodeOp = func(ctx context.Context, a config.OperationConfig) error {
		worker, ok := a.(*PingWorker)
		if !ok {
			return errors.New("invalid input: expected a PingWorker")
		}
		return worker.PingChaincodeAllOrgPeers()
	}
	c.InitFunc = func(id string, a config.OperationConfig) (string, error) {
		worker, ok := a.(*PingWorker)
		if !ok {
			return "", errors.New("invalid input: expected a PingWorker")
		}

		return uuid.New().String(), worker.setupPingWorker()
	}
	c.ShutdownFunc = func(a config.OperationConfig) {
		worker, ok := a.(*PingWorker)
		if !ok {
			return
		}
		worker.Close()
	}

	return nil
}

func (a *PingWorker) setupPingWorker() error {
	// Load SDK config to get info about deployed DLN
	sdk, err := fabsdk.New(fabricConfig.FromFile(sdkConfigPath))
	if err != nil {
		return err
	}

	a.Sdk = sdk

	username, err := sdkUser(a.Sdk, a.Org)
	if err != nil {
		return err
	}

	a.Username = username

	return nil
}

// Close Fabric SDK resources and connections before exiting
func (a *PingWorker) Close() {
	if a.Sdk != nil {
		a.Sdk.Close()
	}
}

// Gets a user defined in the SDK config for the given org
func sdkUser(sdk *fabsdk.FabricSDK, org string) (string, error) {
	conf, err := sdk.Config()
	if err != nil {
		return "", fmt.Errorf("failed to load sdk config: %w", err)
	}

	users, ok := conf.Lookup("organizations." + org + ".users")
	if !ok {
		return "", fmt.Errorf("organization users not found")
	}

	fabUsers, ok := users.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("organization users was not a map")
	}

	if len(fabUsers) == 0 {
		return "", fmt.Errorf("users map empty")
	}

	// Pick just one defined user
	var user string
	for name := range fabUsers {
		user = name
		break
	}

	if user == "" {
		return "", fmt.Errorf("empty user found in sdk config")
	}

	return user, nil
}

func (a *PingWorker) PingChaincodeAllOrgPeers() error {
	logger := log.WithFields(log.Fields{
		"funcName": "PingWorker.PingChaincodeAllorgPeers",
		"org":      a.Org,
	})

	logLevel, err := log.ParseLevel(a.LogLevel)
	if err != nil {
		logger.WithField("logLevel", a.LogLevel).Warn("Unable to parse configured log level. Defaulting to INFO.")
		logLevel = log.DebugLevel
	}
	log.SetLevel(logLevel)

	log.New()

	ctx, err := a.Sdk.Context(
		fabsdk.WithUser(a.Username),
		fabsdk.WithOrg(a.Org))()
	if err != nil {
		logger.WithError(err).Error("error creating context client")
		return fmt.Errorf("error creating context client: %w", err)
	}

	orgPeers, ok := ctx.EndpointConfig().PeersConfig(a.Org)
	if !ok {
		logger.Error("error fetching peers for org")
		return fmt.Errorf("error fetching peers for org: %s", a.Org)
	}

	for _, peer := range orgPeers {
		logger.Infof("pinging chaincode on peer %v...", peer.URL)

		errPing := a.pingChaincode(peer, ctx)
		if errPing != nil {
			logger.WithError(errPing).Errorf("Failed to ping chaincode on peer %v", peer)
			return fmt.Errorf("failed to ping chaincode: %w", errPing)
		}

		logger.Infof("successfully pinged chaincode on peer %v", peer.URL)
	}

	return nil
}
func (a *PingWorker) pingChaincode(targetPeer fab.PeerConfig, ctx contextAPI.Client) error {
	logger := log.WithFields(log.Fields{
		"funcName":  "PingWorker.PingChaincode",
		"channelID": channelID,
		"username":  a.Username,
		"org":       a.Org,
	})

	const retries = 10

	const waitTime = 10 * time.Second

	chClient, err := channel.New(a.Sdk.ChannelContext(
		channelID,
		fabsdk.WithUser(a.Username),
		fabsdk.WithOrg(a.Org),
	))

	testPeer, errPeer := ctx.InfraProvider().CreatePeerFromConfig(&fab.NetworkPeer{PeerConfig: targetPeer})
	if errPeer != nil {
		logger.WithError(errPeer).Error("error fetching peers via infra provider")
		return fmt.Errorf("error fetching peers via infra provider: %w", errPeer)
	}
	// Query PingChaincode, ignoring non-error response
	// Retry if necessary to give the chaincode time to propagate
	for i := 0; i < retries; i++ {
		_, err = chClient.Query(channel.Request{
			ChaincodeID: a.ChaincodeID,
			Fcn:         "pingChaincode",
		}, channel.WithTargets(testPeer))
		if err == nil {
			logger.Info("Successfully queried pingChaincode")
			return nil
		}

		logger.WithField("retry", i).WithError(err).Info("Failed to query pingChaincode, retrying")

		time.Sleep(waitTime)
	}

	logger.WithError(err).Error("Failed to query pingChaincode function")

	return fmt.Errorf("failed to query pingChaincode function: %w", err)
}
