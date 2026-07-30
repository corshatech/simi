package metrics

import (
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"

	"github.com/corshatech/corsha-common/stream"
)

//go:generate mockery --name Manager

// LicenseLabels represent the characteristics of a customer license and any chaincode operations done using that license.
type LicenseLabels struct {
	CustomerID uuid.UUID
	DivisionID uuid.UUID
}

// ActionLabels represent the characteristics of chaincode's action invocations, such as which action was invoked and whether it succeeded.
type ActionLabels struct {
	Action             string
	Success            bool
	ElapsedTimeSeconds float64
}

// InvalidLicenseCause represents the reason why a customer's license was deemed invalid. The different causes can then
// be used to differentiate between customer/user error and system error leading to the license invalidation.
type InvalidLicenseCause string

const (
	BeyondEndOfLicense         InvalidLicenseCause = "beyond_end_of_license"
	BeyondMaxActiveStreams     InvalidLicenseCause = "beyond_max_active_streams"
	CustomerNotActive          InvalidLicenseCause = "customer_not_active"
	DivisionNotActive          InvalidLicenseCause = "division_not_active"
	RootAccountIDIsNil         InvalidLicenseCause = "root_account_id_is_nil"
	CustomerNotFound           InvalidLicenseCause = "customer_not_found" // Root account ID cannot be mapped to licensing data
	CustomerIsEmpty            InvalidLicenseCause = "customer_is_empty"  // Customer ID was in the customers list, but the saved customer was empty
	CustomerListUnmarshalError InvalidLicenseCause = "customer_list_unmarshal_error"
	CustomerUnmarshalError     InvalidLicenseCause = "customer_unmarshal_error"
	DivisionNotFoundInCustomer InvalidLicenseCause = "division_not_found_in_customer"
	SystemError                InvalidLicenseCause = "system_error" // transient issues, theoretically impossible conditions, etc.
)

// Manager manages all the data for Prometheus metrics
type Manager interface {
	IncrementClientStreamCount(labels *LicenseLabels)
	OnFailedStreamChallenge(accountID uuid.UUID, streamID stream.ID)
	RecordChaincodeActionInvocation(labels *ActionLabels)
	IncrementInvalidLicensingEventCount(labels *LicenseLabels, cause InvalidLicenseCause)
}

// NewMetricsManager initializes a new metrics manager
func NewMetricsManager(reg *prometheus.Registry) (Manager, error) {
	// Create a Prometheus counter for the number of client streams per customer and division
	streamsCounter := promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: "streams_total",
			Help: "The total number of client streams",
		},
		[]string{"customerID", "divisionID"},
	)

	actionsHistogram := promauto.With(reg).NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "chaincode_actions",
			Help: "A Histogram of Chaincode Actions called in Invoke",
		},
		[]string{"action", "success", "elapsedTimeSeconds"},
	)

	// Create a Prometheus counter for the number of failed stream challenges
	failedStreamChallengesCounter := promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: "chaincode_failed_stream_challenge_total",
			Help: "The total number of failed stream challenges",
		},
		[]string{"accountID", "streamID"},
	)

	// invalidLicenseCounter is a Prometheus counter for invalid license events
	invalidLicenseCounter := promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: "invalid_license_total",
			Help: "The total number of invalid license events",
		},
		[]string{"customerID", "divisionID", "cause", "isUserError"},
	)

	return &metricsContainer{
		streams:                streamsCounter,
		chaincodeActions:       actionsHistogram,
		invalidLicenseEvents:   invalidLicenseCounter,
		failedStreamChallenges: failedStreamChallengesCounter,
	}, nil
}

// metricsContainer implements Manager interface
type metricsContainer struct {
	chaincodeActions       *prometheus.HistogramVec
	failedStreamChallenges *prometheus.CounterVec
	invalidLicenseEvents   *prometheus.CounterVec
	streams                *prometheus.CounterVec
}

// OnFailedStreamChallenge implements Manager.
func (m *metricsContainer) OnFailedStreamChallenge(accountID uuid.UUID, streamID stream.ID) {
	m.failedStreamChallenges.WithLabelValues(accountID.String(), streamID.String()).Inc()
}

// IncrementClientStreamCount increments the count of client streams using the provided labels
func (m *metricsContainer) IncrementClientStreamCount(labels *LicenseLabels) {
	m.streams.WithLabelValues(labels.CustomerID.String(), labels.DivisionID.String()).Inc()
}

// RecordChaincodeActionInvocation records the invocation of a chaincode action using the provided labels
func (m *metricsContainer) RecordChaincodeActionInvocation(labels *ActionLabels) {
	elapsedTimeSeconds := fmt.Sprintf("%f", labels.ElapsedTimeSeconds)

	m.chaincodeActions.WithLabelValues(labels.Action, strconv.FormatBool(labels.Success), elapsedTimeSeconds).Observe(labels.ElapsedTimeSeconds)
}

// IncrementInvalidLicensingEventCount increments the count of invalid customer license events using the provided labels.
func (m *metricsContainer) IncrementInvalidLicensingEventCount(labels *LicenseLabels, cause InvalidLicenseCause) {
	// We don't always know the customer and division IDs yet when a request is rejected for license-related reasons
	customerIDString, divisionIDString := "", ""
	if labels != nil {
		customerIDString = labels.CustomerID.String()
		divisionIDString = labels.DivisionID.String()
	}

	logger := log.WithFields(log.Fields{
		"func":       "Manager.IncrementInvalidLicensingEventCount",
		"customerID": customerIDString,
		"divisionID": divisionIDString,
		"cause":      cause,
	})

	isUserError := isInvalidLicenseDueToUserError(logger, cause)

	m.invalidLicenseEvents.WithLabelValues(
		customerIDString,
		divisionIDString,
		string(cause),
		strconv.FormatBool(isUserError),
	).Inc()
}

// isInvalidLicenseDueToUserError determines if the given cause for the license invalidation is due to error on the customer's part or ours
func isInvalidLicenseDueToUserError(logger *log.Entry, cause InvalidLicenseCause) bool {
	switch cause {
	case BeyondEndOfLicense, BeyondMaxActiveStreams, CustomerNotActive, DivisionNotActive:
		return true
	case CustomerNotFound, CustomerIsEmpty, CustomerListUnmarshalError, CustomerUnmarshalError, DivisionNotFoundInCustomer, RootAccountIDIsNil:
		// This is definitely our fault, not the customer's fault. We are responsible for maintaining data to map from
		// streams and accounts to licensing data (customers and divisions).
		return false
	case SystemError:
		// This is probably our fault, but it could also just be a transient problem that resolves itself.
		logger.Warning("Unexpected system error caused invalid licensing event! If this is not due to a transient connectivity issue, this requires investigation.")
		return false
	default:
		// This should be impossible. Swallow error and continue because recording metrics should not block normal operation.
		logger.Error("Unexpected cause for invalid licensing event!")
		return false
	}
}
