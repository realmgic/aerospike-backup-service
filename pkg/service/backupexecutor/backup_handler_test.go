package backupexecutor

import (
	"context"
	"errors"
	"testing"

	"github.com/aerospike/aerospike-backup-service/v3/pkg/service/aerospike"
	"github.com/aerospike/backup-go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCombinedBackupHandler_Wait_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	xdrHandler := NewMockBackupHandler(ctrl)
	scanHandler := NewMockBackupHandler(ctrl)

	xdrHandler.EXPECT().Wait(gomock.Any()).Return(nil)
	scanHandler.EXPECT().Wait(gomock.Any()).Return(nil)

	handler := &CombinedBackupHandler{
		xdrHandler:  xdrHandler,
		scanHandler: scanHandler,
	}

	require.NoError(t, handler.Wait(t.Context()))
}

func TestCombinedBackupHandler_Wait_JoinsErrors(t *testing.T) {
	ctrl := gomock.NewController(t)

	xdrErr := errors.New("xdr failed")
	scanErr := errors.New("scan failed")

	xdrHandler := NewMockBackupHandler(ctrl)
	scanHandler := NewMockBackupHandler(ctrl)

	xdrHandler.EXPECT().Wait(gomock.Any()).Return(xdrErr)
	scanHandler.EXPECT().Wait(gomock.Any()).Return(scanErr)

	handler := &CombinedBackupHandler{
		xdrHandler:  xdrHandler,
		scanHandler: scanHandler,
	}

	err := handler.Wait(t.Context())
	require.Error(t, err)
	assert.ErrorContains(t, err, "XDR backup failed")
	assert.ErrorContains(t, err, "scan backup failed")
	assert.ErrorIs(t, err, xdrErr)
	assert.ErrorIs(t, err, scanErr)
}

func TestCombinedBackupHandler_GetStats(t *testing.T) {
	ctrl := gomock.NewController(t)

	xdrStats := models.NewBackupStats()
	xdrStats.TotalRecords.Store(10)
	scanStats := models.NewBackupStats()
	scanStats.TotalRecords.Store(5)

	xdrHandler := NewMockBackupHandler(ctrl)
	scanHandler := NewMockBackupHandler(ctrl)

	xdrHandler.EXPECT().GetStats().Return(xdrStats)
	scanHandler.EXPECT().GetStats().Return(scanStats)

	handler := &CombinedBackupHandler{
		xdrHandler:  xdrHandler,
		scanHandler: scanHandler,
	}

	stats := handler.GetStats()
	require.NotNil(t, stats)
	assert.Equal(t, uint64(15), stats.TotalRecords.Load())
}

func TestCombinedBackupHandler_GetMetrics(t *testing.T) {
	ctrl := gomock.NewController(t)

	xdrMetrics := models.NewMetrics(1, 2, 100, 50)
	scanMetrics := models.NewMetrics(3, 4, 25, 10)

	xdrHandler := NewMockBackupHandler(ctrl)
	scanHandler := NewMockBackupHandler(ctrl)

	xdrHandler.EXPECT().GetMetrics().Return(xdrMetrics)
	scanHandler.EXPECT().GetMetrics().Return(scanMetrics)

	handler := &CombinedBackupHandler{
		xdrHandler:  xdrHandler,
		scanHandler: scanHandler,
	}

	metrics := handler.GetMetrics()
	require.NotNil(t, metrics)
	assert.Equal(t, uint64(125), metrics.RecordsPerSecond)
	assert.Equal(t, uint64(60), metrics.KilobytesPerSecond)
}

func TestCloseOnWaitBackupHandler(t *testing.T) {
	ctrl := gomock.NewController(t)

	inner := NewMockBackupHandler(ctrl)
	client := aerospike.NewMockClient(ctrl)
	clientManager := aerospike.NewMockClientManager(ctrl)

	stats := models.NewBackupStats()
	metrics := models.NewMetrics(1, 2, 42, 10)

	inner.EXPECT().Wait(gomock.Any()).Return(nil)
	inner.EXPECT().GetStats().Return(stats)
	inner.EXPECT().GetMetrics().Return(metrics)
	clientManager.EXPECT().Close(client).Times(1)

	handler := &closeOnWaitBackupHandler{
		inner:         inner,
		client:        client,
		clientManager: clientManager,
	}

	require.NoError(t, handler.Wait(context.Background()))
	assert.Equal(t, stats, handler.GetStats())
	assert.Equal(t, metrics, handler.GetMetrics())

	// Close is invoked exactly once even if Wait is called again.
	inner.EXPECT().Wait(gomock.Any()).Return(nil)
	require.NoError(t, handler.Wait(context.Background()))
}
