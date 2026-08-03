package chaincode

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/peer"
	log "github.com/sirupsen/logrus"
)

type ExperimentData struct {
	// data contents do not matter; they exist only to fill the ledger
	Data []byte `json:"data"`
	// counter is used during pruning in combination with the prunig ratio to determine if a transaction can be pruned
	Counter int `json:"counter"`
}

const (
	experimentKeyPrefix = "osuExperiment"
)

// ExperimentWrite is used to write to the ledger for the purpose of the OSU experiments.
// The write expects three arguments: a key, a counter, and a blob of text.
func ExperimentWrite(stub shim.ChaincodeStubInterface, args []string) peer.Response {
	logger := log.WithField("func", "ExperimentWrite")

	key, counter, data, err := parseExperimentWriteArgs(args)
	if err != nil {
		logger.WithError(err).Error("failed to parse ExperimentWrite args")

		return shim.Error(fmt.Sprintf("failed to parse ExperimentWrite args: %v", err))
	}

	key = experimentKeyPrefix + key

	// create a customer using the customer request.
	experimentData := ExperimentData{
		Counter: counter,
		Data:    data,
	}

	// Save customer bytes to private data collection
	experimentWriteJSON, err := json.Marshal(experimentData)
	if err != nil {
		logger.WithFields(log.Fields{
			"error": err,
		}).Error("failed to marshal fully-specified ExperimentData JSON")

		return shim.Error(fmt.Sprintf("failed to marshal ExperimentData JSON: %v", err))
	}

	if err = stub.PutState(key, experimentWriteJSON); err != nil {
		logger.WithFields(log.Fields{
			"error": err,
		}).Error("failed to put ExperimentData into ledger")

		return shim.Error(fmt.Sprintf("failed to put ExperimentData into ledger: %v", err))
	}

	logger.Infof("Wrote ExperimentData with counter %d to ledger.", counter)

	return shim.Success(nil)
}

func parseExperimentWriteArgs(args []string) (string, int, []byte, error) {
	const numArgs = 3

	if len(args) != numArgs {
		return "", 0, nil, fmt.Errorf("invalid number of arguments: expected 3, got %d", len(args))
	}

	key := args[0]
	data := args[2]

	counter, err := strconv.Atoi(args[1])
	if err != nil {
		return "", 0, nil, err
	}

	return key, counter, []byte(data), nil
}

// ExperimentRead is used to read from the ledger for the purpose of the OSU experiments.
// The write expects one arguments: a key
func ExperimentRead(stub shim.ChaincodeStubInterface, args []string) peer.Response {
	logger := log.WithField("func", "ExperimentRead")

	key, err := parseExperimentReadArgs(args)
	if err != nil {
		logger.WithFields(log.Fields{
			"error": err,
		}).Error("failed to parse ExperimentRead args")

		return shim.Error(fmt.Sprintf("failed to parse ExperimentRead args: %v", err))
	}

	key = experimentKeyPrefix + key

	var rawData []byte

	if rawData, err = stub.GetState(key); err != nil {
		logger.WithFields(log.Fields{
			"error": err,
		}).Error("failed to read ExperimentData from ledger.")

		return shim.Error(fmt.Sprintf("failed to read ExperimentData from ledger: %v", err))
	}

	data := ExperimentData{}

	err = json.Unmarshal(rawData, &data)
	if err != nil {
		logger.WithFields(log.Fields{
			"error": err,
		}).Error("failed to unmarshal ExperimentData")

		return shim.Error(fmt.Sprintf("failed to unmarshal ExperimentData: %v", err))
	}

	logger.Infof("Read ExperimentData counter: %d", data.Counter)

	return shim.Success(nil)
}

func parseExperimentReadArgs(args []string) (string, error) {
	const numArgs = 1

	if len(args) != numArgs {
		return "", fmt.Errorf("invalid number of arguments: expected 1, got %d", len(args))
	}

	return args[0], nil
}
