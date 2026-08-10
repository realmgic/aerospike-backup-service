package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aerospike/aerospike-backup-service/v3/pkg/model"
	"github.com/aerospike/aerospike-backup-service/v3/pkg/util/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestNewIPRateLimiter(t *testing.T) {
	t.Parallel()

	limiter := NewIPRateLimiter(rate.Limit(5), 10)
	require.NotNil(t, limiter)
	assert.Empty(t, limiter.limiters)
}

func TestIPRateLimiter_AddLimiter(t *testing.T) {
	t.Parallel()

	limiter := NewIPRateLimiter(rate.Limit(5), 10)
	l := limiter.AddLimiter("1.2.3.4")
	require.NotNil(t, l)
	assert.Same(t, l, limiter.limiters[IPAddress("1.2.3.4")])
}

func TestIPRateLimiter_GetLimiter_CreatesAndReuses(t *testing.T) {
	t.Parallel()

	limiter := NewIPRateLimiter(rate.Limit(5), 10)

	first := limiter.GetLimiter("1.2.3.4")
	require.NotNil(t, first)

	second := limiter.GetLimiter("1.2.3.4")
	assert.Same(t, first, second)

	other := limiter.GetLimiter("5.6.7.8")
	assert.NotSame(t, first, other)
}

func TestNewIPWhiteList(t *testing.T) {
	t.Parallel()

	wl := newIPWhiteList([]string{"10.0.0.1", "192.168.1.0/24"})
	require.Len(t, wl.addresses, 1)
	require.Len(t, wl.networks, 1)
	assert.False(t, wl.allowAny)
}

func TestNewIPWhiteList_AllowAny(t *testing.T) {
	t.Parallel()

	wl := newIPWhiteList([]string{"0.0.0.0/0"})
	assert.True(t, wl.allowAny)
	assert.Empty(t, wl.addresses)
	assert.Empty(t, wl.networks)
}

func TestNewIPWhiteList_InvalidIP_Panics(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		newIPWhiteList([]string{"not-an-ip"})
	})
}

func TestIPWhiteList_IsAllowed(t *testing.T) {
	t.Parallel()

	wl := newIPWhiteList([]string{"10.0.0.1", "192.168.1.0/24"})

	assert.True(t, wl.isAllowed("10.0.0.1"))
	assert.True(t, wl.isAllowed("192.168.1.42"))
	assert.False(t, wl.isAllowed("10.0.0.2"))
	assert.False(t, wl.isAllowed("not-an-ip"))
}

func TestIPWhiteList_IsAllowed_AllowAny(t *testing.T) {
	t.Parallel()

	wl := newIPWhiteList([]string{"0.0.0.0/0"})
	assert.True(t, wl.isAllowed("anything-goes-since-allowAny-short-circuits"))
}

func TestRateLimiter_Middleware(t *testing.T) {
	t.Parallel()

	config := &model.RateLimiterConfig{
		Tps:       ptr.Of(1),
		Size:      ptr.Of(1),
		WhiteList: []string{"9.9.9.9"},
	}

	handlerCalls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalls++
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimiter(config)(next)

	// Whitelisted IP always bypasses the limiter.
	for range 3 {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
	assert.Equal(t, 3, handlerCalls)

	// Non-whitelisted IP: first request within burst succeeds, second is rate-limited.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "1.2.3.4:1234"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "1.2.3.4:5678"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

func TestRateLimiter_Middleware_InvalidRemoteAddr(t *testing.T) {
	t.Parallel()

	config := &model.RateLimiterConfig{Tps: ptr.Of(100), Size: ptr.Of(100)}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := RateLimiter(config)(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "not-a-valid-remote-addr"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
