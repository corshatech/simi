package metrics //nolint:testpackage

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestInvalidLicenseEventCauseIsUserError(t *testing.T) {
	for _, test := range []struct {
		description     string
		cause           InvalidLicenseCause
		wantIsUserError bool
	}{
		{
			description:     "license is expired",
			cause:           BeyondEndOfLicense,
			wantIsUserError: true,
		},
		{
			description:     "license max streams exceeded",
			cause:           BeyondMaxActiveStreams,
			wantIsUserError: true,
		},
		{
			description:     "customer is deactivated",
			cause:           CustomerNotActive,
			wantIsUserError: true,
		},
		{
			description:     "division is deactivated",
			cause:           DivisionNotActive,
			wantIsUserError: true,
		},
		{
			description:     "root account ID is nil",
			cause:           RootAccountIDIsNil,
			wantIsUserError: false,
		},
		{
			description:     "customer not in customer list",
			cause:           CustomerNotFound,
			wantIsUserError: false,
		},
		{
			description:     "customer in customer list but does not exist",
			cause:           CustomerIsEmpty,
			wantIsUserError: false,
		},
		{
			description:     "customer list invalid",
			cause:           CustomerListUnmarshalError,
			wantIsUserError: false,
		},
		{
			description:     "customer invalid",
			cause:           CustomerUnmarshalError,
			wantIsUserError: false,
		},
		{
			description:     "division not found in customer",
			cause:           DivisionNotFoundInCustomer,
			wantIsUserError: false,
		},
		{
			description:     "system error occurred",
			cause:           SystemError,
			wantIsUserError: false,
		},
		{
			description:     "unexpected cause",
			cause:           "not a valid cause!!!!",
			wantIsUserError: false,
		},
	} {
		t.Run(test.description, func(t *testing.T) {
			logger := log.WithField("func", "TestInvalidLicenseEventCauseIsUserError")

			gotIsUserError := isInvalidLicenseDueToUserError(logger, test.cause)
			require.Equal(t, test.wantIsUserError, gotIsUserError)
		})
	}
}
