package chaincode_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/corshatech/fabric-chaincode/chaincode/internal/chaincode"
	"github.com/corshatech/fabric-chaincode/chaincode/internal/chaincode/metrics"
)

func TestNewChaincode(t *testing.T) {
	// create clientset
	reg := prometheus.NewRegistry()

	mm, err := metrics.NewMetricsManager(reg)
	require.NoError(t, err)

	cc := chaincode.NewChaincode(mm)

	assert.NotNil(t, cc)
}
