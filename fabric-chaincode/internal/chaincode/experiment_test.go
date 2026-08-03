package chaincode_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/stretchr/testify/require"

	"github.com/corshatech/fabric-chaincode/chaincode/internal/chaincode"
)

func TestExperimentWrite(t *testing.T) {
	mockData := "aaaa"

	tests := []struct {
		name              string
		args              []string
		putStateCallCount int
		putStateError     bool
		shimError         bool
	}{
		{
			name:              "Happy path",
			args:              []string{"0", "2", mockData},
			putStateError:     false,
			putStateCallCount: 1,
			shimError:         false,
		},
		{
			name:              "Put state errors",
			args:              []string{"0", "2", mockData},
			putStateError:     true,
			putStateCallCount: 1,
			shimError:         true,
		},
		{
			name:              "invalid args",
			args:              nil,
			putStateError:     true,
			putStateCallCount: 0,
			shimError:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStub := &mock.ChaincodeStub{}

			mockStub.PutStateStub = func(key string, data []byte) error {
				if tt.putStateError {
					return errors.New("error with putstate")
				}

				return nil
			}

			// Test good case
			peerResponse := chaincode.ExperimentWrite(mockStub, tt.args)

			if tt.shimError {
				require.Equal(t, int32(shim.ERROR), peerResponse.GetStatus())
			} else {
				require.Equal(t, int32(shim.OK), peerResponse.GetStatus())
			}

			require.Equal(t, tt.putStateCallCount, mockStub.PutStateCallCount())

			if tt.putStateCallCount < 1 {
				return
			}

			key, dataBytes := mockStub.PutStateArgsForCall(0)

			require.Equal(t, "osuExperiment0", key)

			data := chaincode.ExperimentData{}

			err := json.Unmarshal(dataBytes, &data)
			require.NoError(t, err)

			require.Equal(t, 2, data.Counter)
			require.Equal(t, mockData, string(data.Data))
		})
	}
}

func TestExperimentRead(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		getStateCallCount int
		getStateError     bool
		shimError         bool
	}{
		{
			name:              "Happy path",
			args:              []string{"0"},
			getStateError:     false,
			getStateCallCount: 1,
			shimError:         false,
		},
		{
			name:              "get state errors",
			args:              []string{"0"},
			getStateError:     true,
			getStateCallCount: 1,
			shimError:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStub := &mock.ChaincodeStub{}

			experimentData := chaincode.ExperimentData{
				Counter: 2,
				Data:    []byte("bbbb"),
			}

			experimentWriteJSON, err := json.Marshal(experimentData)
			require.NoError(t, err)

			mockStub.GetStateStub = func(key string) ([]byte, error) {
				if tt.getStateError {
					return nil, errors.New("error with putstate")
				}

				return experimentWriteJSON, nil
			}

			peerResponse := chaincode.ExperimentRead(mockStub, tt.args)

			if tt.shimError {
				require.Equal(t, int32(shim.ERROR), peerResponse.GetStatus())
			} else {
				require.Equal(t, int32(shim.OK), peerResponse.GetStatus())
			}

			require.Equal(t, tt.getStateCallCount, mockStub.GetStateCallCount())

			if tt.getStateCallCount < 1 {
				return
			}

			key := mockStub.GetStateArgsForCall(0)
			require.Equal(t, "osuExperiment0", key)
		})
	}
}
