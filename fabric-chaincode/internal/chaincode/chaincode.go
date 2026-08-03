package chaincode

import (
	"time"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/peer"
	log "github.com/sirupsen/logrus"

	corshaErrors "github.com/corshatech/corsha-common/errors"
	"github.com/corshatech/fabric-chaincode/chaincode/internal/chaincode/metrics"
)

// Chaincode implements the chaincode interface for managing customer licenses
type Chaincode struct {
	// Metrics holds all the collectors for chaincode's Prometheus data
	Metrics metrics.Manager
}

func NewChaincode(metrics metrics.Manager) *Chaincode {
	return &Chaincode{
		Metrics: metrics,
	}
}

// Init initializes the chaincode
func (*Chaincode) Init(_ shim.ChaincodeStubInterface) peer.Response {
	return shim.Success(nil)
}

// Invoke is the entrypoint for chaincode invocations
func (cc *Chaincode) Invoke(stub shim.ChaincodeStubInterface) peer.Response {
	funcName, args := stub.GetFunctionAndParameters()

	var peerResponse peer.Response

	start := time.Now()

	logger := log.WithFields(log.Fields{
		"funcName": funcName,
		"args":     args,
	})

	logger.Debug("Invoke called")

	switch funcName {
	case "pingChaincode":
		peerResponse = pingChaincode(stub, args)
	case "experimentWrite":
		peerResponse = ExperimentWrite(stub, args)
	case "experimentRead":
		peerResponse = ExperimentRead(stub, args)
	default:
		logger.Warn("Unsupported chaincode function")
		return shim.Error(corshaErrors.DLNFunctionNotFoundError.Error())
	}

	timeNow := time.Since(start)
	success := true

	if peerResponse.GetStatus() != shim.OK {
		logger.WithField("message", peerResponse.GetMessage()).Warn("Invoke failed")

		success = false
	}

	logger.Debug("Updating histogram for chaincode actions")
	cc.Metrics.RecordChaincodeActionInvocation(&metrics.ActionLabels{Action: funcName, ElapsedTimeSeconds: timeNow.Seconds(), Success: success})

	return peerResponse
}

// pingChaincode is used by healthcheck to make sure the chaincode is constantly running
func pingChaincode(_ shim.ChaincodeStubInterface, _ []string) peer.Response {
	return shim.Success(nil)
}
