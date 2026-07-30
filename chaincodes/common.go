package chaincodes

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/channel"
	// "github.com/hyperledger/fabric-sdk-go/pkg/client/msp"
	// fabricConfig "github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
	"github.com/iancoleman/strcase"
	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"github.com/corshatech/simi/config"
)

const (
	channelID     = "firstchannel"
	sdkConfigPath = "/fabric-config/fabric-config.yaml"
	org           = "Org1MSP"
	timeout       = 3000 * time.Second
)

// sdkUser extracts a valid user identity from the Fabric SDK configuration for the specified
// organization. This function navigates the SDK's configuration structure to locate user
// definitions within the organization's configuration section and selects the first available
// user for authentication purposes. The selected user must have valid certificates and
// permissions to perform chaincode operations on behalf of the organization.
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

// CCWorker represents a Hyperledger Fabric client worker that executes a custom chaincode
// operation to increment a counter value stored on each peer. This worker maintains connections
// to the Fabric network and provides the necessary configuration and SDK instances to interact
// with deployed chaincode.
//
// ChaincodeID identifies the target chaincode, Sdk provides network connectivity, Username
// and Org handle authentication, and LogLevel controls debugging output.
type CCWorker struct {
	ChaincodeID string            `yaml:"chaincodeID"` // Identifier of the deployed chaincode to invoke
	Sdk         *fabsdk.FabricSDK // Fabric SDK instance for network communication
	chClient    *channel.Client   // client for communicating to chaincodes
	Username    string            `yaml:"username"` // Username for authentication with the Fabric network
	Org         string            `yaml:"org"`      // Organization name this worker belongs to
	LogLevel    string            `yaml:"logLevel"` // Logging level for debugging and monitoring
	Id          string            // Id is the key used to control where the worker reads / writes from
	Counter     int               // used to help correlate the pruning of transactions
}

// setupCCWorker initializes the Fabric SDK and prepares the worker for chaincode operations.
// This method loads the Fabric network configuration from the specified config file, establishes
// the SDK instance that will be used for all subsequent network communications, and determines
// the appropriate user credentials for authentication. The setup process is critical for ensuring
// that the worker can successfully connect to and interact with the Hyperledger Fabric network.
func (a *CCWorker) setupCCWorker() error {
	return nil
}

func (a *CCWorker) Close() {
}

func SetupSimi(c *config.WorkerConfig, numWriters int) error {
	logger := log.WithFields(log.Fields{
		"funcName": "admin.SetupSimi",
	})
	var worker *CCWorker
	configMetadata := mapstructure.Metadata{}

	// Decode raw interface into ProxyConfiguration struct and metadata
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,            // allows weak conversions described here: https://pkg.go.dev/github.com/mitchellh/mapstructure@v1.5.0#Decoder.Decode
		Result:           &worker,         // CCWorker struct
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
		logger.Errorf("Failed to decode worker: %v", err)
		logger.Debugf("Metadata.Unused: %#v", configMetadata.Unused)
		logger.Debugf("Metadata.Unset: %#v", configMetadata.Unset)
		return err
	}
	c.OperationConfig = worker
	c.InitFunc = func(id string, a config.OperationConfig) (string, error) {
		worker, ok := a.(*CCWorker)
		if !ok {
			return "", errors.New("invalid input: expected a CCWorker")
		}

		err := worker.setupCCWorker()
		if err != nil {
			return "", err
		}

		if c.OperationType == "read" {
			idInt, err := strconv.Atoi(worker.Id)
			if err != nil {
				log.Fatalln("Failed tp parse ID")
			}

			newId := idInt % numWriters
			worker.Id = strconv.Itoa(newId)
		}

		return uuid.New().String(), nil
	}
	c.ShutdownFunc = func(a config.OperationConfig) {
		worker, ok := a.(*CCWorker)
		if !ok {
			return
		}
		worker.Close()
	}

	return nil
}

type fabricPeers struct {
	Peers map[string]struct {
		URL string `yaml:"url" json:"url"`
	} `yaml:"peers"`
}

func (a *CCWorker) randomPeer() (string, error) {
	f, err := os.Open(sdkConfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to open fabric config: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("failed to read fabric config: %w", err)
	}

	var cfg fabricPeers
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse fabric config: %w", err)
	}

	if len(cfg.Peers) == 0 {
		return "", fmt.Errorf("no peers found in config")
	}

	keys := make([]string, 0, len(cfg.Peers))
	for k := range cfg.Peers {
		keys = append(keys, k)
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(keys))))
	if err != nil {
		return "", fmt.Errorf("failed to pick random peer: %w", err)
	}

	return keys[n.Int64()], nil
}

// invokeChaincodeCli executes a Hyperledger Fabric chaincode function using the kubectl hlf CLI.
func (a *CCWorker) invokeChaincodeCli(verb string, fcn string, args [][]byte) (string, error) {
	logger := log.WithFields(log.Fields{
		"funcName":  "CCWorker.invokeChaincodeCli",
		"chaincode": a.ChaincodeID,
		"channelID": channelID,
		"fnc":       fcn,
		"verb":      verb,
	})

	peer, err := a.randomPeer()
	logger = logger.WithField("peer", peer)

	if err != nil {
		logger.WithError(err).Error("Failed to select random peer")
		return peer, err
	} else {
		logger.Warnf("Selected random peer %s", peer)
	}

	// 1. Start building the CLI command arguments
	cmdArgs := []string{
		"hlf",
		"chaincode",
		verb,
		"--config=/fabric-config/fabric-config.yaml", // Use the mounted config file
		"--user=admin",
		fmt.Sprintf("--peer=%s", peer),
		fmt.Sprintf("--chaincode=%s", a.ChaincodeID),
		fmt.Sprintf("--channel=%s", channelID),
		fmt.Sprintf("--fcn=%s", fcn),
	}

	// 2. Add Arguments using the '-a' flag structure
	for _, argBytes := range args {
		argString := string(argBytes)

		// Add both the '-a' flag and the argument to the command list
		cmdArgs = append(cmdArgs, "-a", argString)
	}

	// 3. Execute the Command
	cmd := exec.Command("kubectl", cmdArgs...)

	// Capture output for debugging (stdout and stderr)
	output, err := cmd.CombinedOutput()
	outputString := string(output)

	if err != nil {
		cmdString := cmd.String()
		logger.WithFields(log.Fields{
			"output": outputString,
			"cmd":    cmdString,
		}).Error("HLF CLI invocation failed")
		// Return a detailed error including the CLI output
		return peer, fmt.Errorf("HLF CLI invocation failed: %s, output: %s", err, outputString)
	}

	logger.WithFields(log.Fields{
		"output": outputString,
	}).Info("HLF CLI invocation succeeded")

	return peer, nil
}
