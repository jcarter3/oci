package ociregistry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errorTests = []struct {
	testName              string
	err                   error
	wantMsg               string
	wantMarshalData       rawJSONMessage
	wantMarshalHTTPStatus int
}{{
	testName:              "RegularGoError",
	err:                   fmt.Errorf("unknown error"),
	wantMsg:               "unknown error",
	wantMarshalData:       `{"errors":[{"code":"UNKNOWN","message":"unknown error"}]}`,
	wantMarshalHTTPStatus: http.StatusInternalServerError,
}, {
	testName:              "RegistryError",
	err:                   ErrBlobUnknown,
	wantMsg:               "blob unknown: blob unknown to registry",
	wantMarshalData:       `{"errors":[{"code":"BLOB_UNKNOWN","message":"blob unknown to registry"}]}`,
	wantMarshalHTTPStatus: http.StatusNotFound,
}, {
	testName:              "WrappedRegistryErrorWithContextAtStart",
	err:                   fmt.Errorf("some context: %w", ErrBlobUnknown),
	wantMsg:               "some context: blob unknown: blob unknown to registry",
	wantMarshalData:       `{"errors":[{"code":"BLOB_UNKNOWN","message":"some context: blob unknown: blob unknown to registry"}]}`,
	wantMarshalHTTPStatus: http.StatusNotFound,
}, {
	testName:              "WrappedRegistryErrorWithContextAtEnd",
	err:                   fmt.Errorf("%w: some context", ErrBlobUnknown),
	wantMsg:               "blob unknown: blob unknown to registry: some context",
	wantMarshalData:       `{"errors":[{"code":"BLOB_UNKNOWN","message":"blob unknown to registry: some context"}]}`,
	wantMarshalHTTPStatus: http.StatusNotFound,
}, {
	testName: "HTTPStatusIgnoredWithKnownCode",
	err:      NewHTTPError(fmt.Errorf("%w: some context", ErrBlobUnknown), http.StatusUnauthorized, nil, nil),
	wantMsg:  "401 Unauthorized: blob unknown: blob unknown to registry: some context",
	// Note: the "401 Unauthorized" remains intact because it's not redundant with respect
	// to the 404 HTTP response code.
	wantMarshalData:       `{"errors":[{"code":"BLOB_UNKNOWN","message":"401 Unauthorized: blob unknown: blob unknown to registry: some context"}]}`,
	wantMarshalHTTPStatus: http.StatusNotFound,
}, {
	testName:              "HTTPStatusUsedWithUnknownCode",
	err:                   NewHTTPError(NewError("a message with a code", "SOME_CODE", nil), http.StatusUnauthorized, nil, nil),
	wantMsg:               "401 Unauthorized: some code: a message with a code",
	wantMarshalData:       `{"errors":[{"code":"SOME_CODE","message":"a message with a code"}]}`,
	wantMarshalHTTPStatus: http.StatusUnauthorized,
}, {
	testName:              "ErrorWithDetail",
	err:                   NewError("a message with some detail", "SOME_CODE", json.RawMessage(`{"foo": true}`)),
	wantMsg:               `some code: a message with some detail`,
	wantMarshalData:       `{"errors":[{"code":"SOME_CODE","message":"a message with some detail","detail":{"foo":true}}]}`,
	wantMarshalHTTPStatus: http.StatusInternalServerError,
}}

func TestError(t *testing.T) {
	for _, test := range errorTests {
		t.Run(test.testName, func(t *testing.T) {
			assert.EqualError(t, test.err, test.wantMsg)
			data, httpStatus := MarshalError(test.err)
			assert.Equal(t, test.wantMarshalHTTPStatus, httpStatus)
			assertJSONEqual(t, test.wantMarshalData, rawJSONMessage(data), "marshal data: %s", data)

			// Check that the marshaled error unmarshals into WireError OK and
			// that the code matches appropriately.
			var errs *WireErrors
			err := json.Unmarshal(data, &errs)
			require.NoError(t, err)
			if ociErr := Error(nil); errors.As(test.err, &ociErr) {
				require.True(t, errors.Is(errs, NewError("something", ociErr.Code(), nil)))
			}
		})
	}
}

// assertJSONEqual compares two values by marshaling them to JSON and comparing the results.
func assertJSONEqual(t *testing.T, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	expectedJSON, err := json.Marshal(expected)
	require.NoError(t, err, "failed to marshal expected value")
	actualJSON, err := json.Marshal(actual)
	require.NoError(t, err, "failed to marshal actual value")
	assert.JSONEq(t, string(expectedJSON), string(actualJSON), msgAndArgs...)
}

type rawJSONMessage string

func (m rawJSONMessage) MarshalJSON() ([]byte, error) {
	return []byte(m), nil
}

func (m *rawJSONMessage) UnmarshalJSON(data []byte) error {
	*m = rawJSONMessage(data)
	return nil
}
