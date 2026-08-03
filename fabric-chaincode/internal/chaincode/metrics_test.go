package chaincode_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/corshatech/corsha-common/stream"
	"github.com/corshatech/fabric-chaincode/chaincode/internal/chaincode/metrics"
)

func TestNewMetricsManger(t *testing.T) {
	accountID := "b400e41b-2ed3-41b8-9a2c-19863b59a201"
	customerID := "aa5258c7-c8ee-4a2b-ac48-5bf417fd69eb"
	divisionID := "6ecdafdd-4dbb-4959-a9a8-b4015cafd934"

	var streamID stream.ID
	err := streamID.UnmarshalText([]byte("Ari9zNIYusDZFFRpqgP0zTt8aEy7GvdAgMuVoZTNfXdR"))
	require.NoError(t, err)

	reg := prometheus.NewRegistry()
	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg})

	mm, err := metrics.NewMetricsManager(reg)
	require.NoError(t, err)

	t.Run("IncrementClientStreamCount", func(t *testing.T) {
		// Increment the stream count
		mm.IncrementClientStreamCount(&metrics.LicenseLabels{
			CustomerID: uuid.MustParse(customerID),
			DivisionID: uuid.MustParse(divisionID),
		})

		w := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)

		handler.ServeHTTP(w, request)

		res := w.Result()
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		assert.Contains(t, string(data), "\nstreams_total{customerID=\"aa5258c7-c8ee-4a2b-ac48-5bf417fd69eb\",divisionID=\"6ecdafdd-4dbb-4959-a9a8-b4015cafd934\"} 1\n")
	})

	t.Run("OnFailedStreamChallenge", func(t *testing.T) {
		mm.OnFailedStreamChallenge(uuid.MustParse(accountID), streamID)

		w := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)

		handler.ServeHTTP(w, request)

		res := w.Result()
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		assert.Contains(t, string(data), "\nchaincode_failed_stream_challenge_total{accountID=\"b400e41b-2ed3-41b8-9a2c-19863b59a201\",streamID=\"Ari9zNIYusDZFFRpqgP0zTt8aEy7GvdAgMuVoZTNfXdR\"} 1\n")
	})

	t.Run("RecordChaincodeActionInvocation", func(t *testing.T) {
		mm.RecordChaincodeActionInvocation(&metrics.ActionLabels{
			Action:             "MetricsTest",
			Success:            true,
			ElapsedTimeSeconds: 1.312,
		})

		w := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)

		handler.ServeHTTP(w, request)

		res := w.Result()
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		assert.Contains(t, string(data), `# TYPE chaincode_actions histogram
chaincode_actions_bucket{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true",le="0.005"} 0
chaincode_actions_bucket{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true",le="0.01"} 0
chaincode_actions_bucket{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true",le="0.025"} 0
chaincode_actions_bucket{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true",le="0.05"} 0
chaincode_actions_bucket{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true",le="0.1"} 0
chaincode_actions_bucket{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true",le="0.25"} 0
chaincode_actions_bucket{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true",le="0.5"} 0
chaincode_actions_bucket{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true",le="1"} 0
chaincode_actions_bucket{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true",le="2.5"} 1
chaincode_actions_bucket{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true",le="5"} 1
chaincode_actions_bucket{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true",le="10"} 1
chaincode_actions_bucket{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true",le="+Inf"} 1
chaincode_actions_sum{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true"} 1.312
chaincode_actions_count{action="MetricsTest",elapsedTimeSeconds="1.312000",success="true"} 1
`)
	})
}
