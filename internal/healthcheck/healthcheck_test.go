package healthcheck_test

import (
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vladyslavpavlenko/pheme/internal/healthcheck"
	"github.com/vladyslavpavlenko/pheme/internal/logger"
)

func TestHealthyEndpoint(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rt := new(transportMock)

		rt.On("RoundTrip", mock.Anything).
			Return(&http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil)

		hc := healthcheck.New("http://fake/health", 100*time.Millisecond, logger.NewNop(), healthcheck.WithTransport(rt))
		hc.Start()
		defer hc.Stop()

		synctest.Wait()
		assert.Equal(t, healthcheck.StatusHealthy, hc.Status())
	})
}

func TestUnhealthyEndpoint(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rt := new(transportMock)

		rt.On("RoundTrip", mock.Anything).
			Return(&http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}, nil)

		hc := healthcheck.New("http://fake/health", 100*time.Millisecond, logger.NewNop(), healthcheck.WithTransport(rt))
		hc.Start()
		defer hc.Stop()

		synctest.Wait()
		assert.Equal(t, healthcheck.StatusUnhealthy, hc.Status())
	})
}

func TestTransitionToHealthy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rt := new(transportMock)

		rt.On("RoundTrip", mock.Anything).
			Return(&http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}, nil).Once()
		rt.On("RoundTrip", mock.Anything).
			Return(&http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil)

		hc := healthcheck.New("http://fake/health", 100*time.Millisecond, logger.NewNop(), healthcheck.WithTransport(rt))
		hc.Start()
		defer hc.Stop()

		synctest.Wait()
		assert.Equal(t, healthcheck.StatusUnhealthy, hc.Status())

		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		assert.Equal(t, healthcheck.StatusHealthy, hc.Status())
	})
}

type transportMock struct {
	mock.Mock
}

func (m *transportMock) RoundTrip(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}
