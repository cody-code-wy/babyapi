package babytest

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/calvinmclean/babyapi"
	"github.com/stretchr/testify/require"
)

// TestCase is a single test step that executes the provided ClientRequest or RequestFunc and compares to the
// ExpectedResponse
type TestCase[T babyapi.Resource] struct {
	Name string

	// Test is the runnable test to execute before assertions
	Test Test[T]

	// ClientName is the name of the API, or child API, which should be used to execute this test. Leave empty
	// to use the default provided API Client. When set, CreateClientMap is used to create a map of child clients
	// This is only available for TableTest because it has the ClientMap
	ClientName string

	// Assert allows setting a function for custom assertions after making a request. It is part of the Test instead
	// of the ExpectedResponse because it needs the type parameter
	Assert func(*babyapi.Response[T])

	// Expected response to compare
	ExpectedResponse
}

// Test is an interface that allows executing different types of tests before running assertions
type Test[T babyapi.Resource] interface {
	Run(t *testing.T, client *babyapi.Client[T], getResponse PreviousResponseGetter) (*babyapi.Response[T], error)
}

// ExpectedResponse sets up the expectations when running a test
type ExpectedResponse struct {
	// NoBody sets the expectation that the response will have an empty body. This is used because leaving Body
	// empty will just skip the test, not assert the response is empty
	NoBody bool
	// Body is the expected response body string
	Body string
	// BodyRegexp allows comparing a request body by regex
	BodyRegexp string
	// Status is the expected HTTP response code
	Status int
	// Error is an expected error string to be returned by the client
	Error string
}

// Run will execute a Test using t.Run to run with the test name. The API is expected to already be running. If your
// test uses any PreviousResponseGetter, it will have a nil panic since that is used only for TableTest
func (tt TestCase[T]) Run(t *testing.T, client *babyapi.Client[T]) {
	t.Run(tt.Name, func(t *testing.T) {
		if tt.ClientName != "" {
			t.Errorf("cannot use ClientName field when executing without TableTest")
			return
		}

		_ = tt.run(t, client, nil)
	})
}

func (tt TestCase[T]) run(t *testing.T, client *babyapi.Client[T], getResponse PreviousResponseGetter) *babyapi.Response[T] {
	r, err := tt.Test.Run(t, client, getResponse)

	tt.assertError(t, err)
	tt.assertBody(t, r)

	if tt.Assert != nil {
		tt.Assert(r)
	}

	return r
}

func (tt TestCase[T]) assertError(t *testing.T, err error) {
	if tt.ExpectedResponse.Error == "" {
		require.NoError(t, err)
		return
	}

	require.Error(t, err)
	require.Equal(t, tt.ExpectedResponse.Error, err.Error())

	var errResp *babyapi.ErrResponse
	if errors.As(err, &errResp) {
		require.Equal(t, tt.ExpectedResponse.Status, errResp.HTTPStatusCode)
	}
}

func (tt TestCase[T]) assertBody(t *testing.T, r *babyapi.Response[T]) {
	switch {
	case tt.NoBody:
		require.Equal(t, http.NoBody, r.Response.Body)
		require.Equal(t, "", r.Body)
	case tt.BodyRegexp != "":
		require.Regexp(t, tt.ExpectedResponse.BodyRegexp, strings.TrimSpace(r.Body))
	case tt.Body != "":
		if r == nil {
			t.Error("response is nil")
			return
		}
		require.Equal(t, tt.ExpectedResponse.Body, strings.TrimSpace(r.Body))
	}
}
