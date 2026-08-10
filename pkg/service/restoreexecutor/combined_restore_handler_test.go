package restoreexecutor

import (
	"context"
	"errors"
	"testing"

	"github.com/aerospike/backup-go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestNewCombinedRestoreHandler_SkipsNilHandlers(t *testing.T) {
	ctrl := gomock.NewController(t)

	valid := NewMockRestoreHandler(ctrl)
	valid.EXPECT().GetStats().Return(models.NewRestoreStats()).AnyTimes()

	handler := NewCombinedRestoreHandler(nil, valid, nil)
	require.NotNil(t, handler)
	assert.Len(t, handler.handlers, 1)
}

func TestCombinedRestoreHandler_Wait_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	first := NewMockRestoreHandler(ctrl)
	second := NewMockRestoreHandler(ctrl)

	first.EXPECT().Wait(gomock.Any()).Return(nil)
	second.EXPECT().Wait(gomock.Any()).Return(nil)

	handler := NewCombinedRestoreHandler(first, second)
	require.NoError(t, handler.Wait(t.Context()))
}

func TestCombinedRestoreHandler_Wait_Error(t *testing.T) {
	ctrl := gomock.NewController(t)

	restoreErr := errors.New("restore failed")
	handlerMock := NewMockRestoreHandler(ctrl)
	handlerMock.EXPECT().Wait(gomock.Any()).Return(restoreErr)

	handler := NewCombinedRestoreHandler(handlerMock)

	err := handler.Wait(context.Background())
	require.Error(t, err)
	assert.ErrorContains(t, err, "restore failed")
	assert.ErrorIs(t, err, restoreErr)
}

func TestCombinedRestoreHandler_GetStats(t *testing.T) {
	ctrl := gomock.NewController(t)

	firstStats := models.NewRestoreStats()
	firstStats.TotalBytesRead.Store(3)
	secondStats := models.NewRestoreStats()
	secondStats.TotalBytesRead.Store(7)

	first := NewMockRestoreHandler(ctrl)
	second := NewMockRestoreHandler(ctrl)

	first.EXPECT().GetStats().Return(firstStats)
	second.EXPECT().GetStats().Return(secondStats)

	handler := NewCombinedRestoreHandler(first, second)

	stats := handler.GetStats()
	require.NotNil(t, stats)
	assert.Equal(t, uint64(10), stats.GetTotalBytesRead())
}

func TestCombinedRestoreHandler_GetMetrics(t *testing.T) {
	ctrl := gomock.NewController(t)

	first := NewMockRestoreHandler(ctrl)
	second := NewMockRestoreHandler(ctrl)

	first.EXPECT().GetMetrics().Return(models.NewMetrics(1, 2, 100, 50))
	second.EXPECT().GetMetrics().Return(models.NewMetrics(3, 4, 25, 10))

	handler := NewCombinedRestoreHandler(first, second)

	metrics := handler.GetMetrics()
	require.NotNil(t, metrics)
	assert.Equal(t, uint64(125), metrics.RecordsPerSecond)
	assert.Equal(t, uint64(60), metrics.KilobytesPerSecond)
}
