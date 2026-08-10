package restoreexecutor

import (
	"context"
	"errors"
	"testing"

	"github.com/aerospike/aerospike-backup-service/v3/pkg/model"
	"github.com/aerospike/aerospike-backup-service/v3/pkg/service/aerospike"
	"github.com/aerospike/backup-go"
	"github.com/aerospike/backup-go/io/storage/common"
	"github.com/aerospike/backup-go/io/storage/options"
	bgmocks "github.com/aerospike/backup-go/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type mockStorageReader struct {
	reader backup.StreamingReader
	err    error
	calls  int
}

func (m *mockStorageReader) CreateDirReader(
	_ context.Context,
	_ model.Storage,
	_ string,
	_ ...options.Opt,
) (backup.StreamingReader, error) {
	m.calls++
	return m.reader, m.err
}

type storageReadResult struct {
	reader backup.StreamingReader
	err    error
}

type sequentialStorageReader struct {
	results []storageReadResult
	calls   int
}

func (s *sequentialStorageReader) CreateDirReader(
	_ context.Context,
	_ model.Storage,
	_ string,
	_ ...options.Opt,
) (backup.StreamingReader, error) {
	idx := s.calls
	s.calls++
	if idx >= len(s.results) {
		return nil, errors.New("unexpected CreateDirReader call")
	}

	result := s.results[idx]
	return result.reader, result.err
}

func TestDefaultRestoreExecutor_Run_ScanSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)

	reader := bgmocks.NewMockStreamingReader(t)
	restoreHandler := NewMockRestoreHandler(ctrl)

	client := aerospike.NewMockClient(ctrl)
	client.EXPECT().
		Restore(gomock.Any(), gomock.Any(), reader).
		Return(restoreHandler, nil)

	operations := &sequentialStorageReader{
		results: []storageReadResult{
			{reader: reader, err: nil},
			{err: common.ErrEmptyStorage},
		},
	}
	executor := NewRestore(operations)

	handler, err := executor.Run(t.Context(), client, testRestoreRequest())
	require.NoError(t, err)
	require.NotNil(t, handler)
	assert.Equal(t, 2, operations.calls)
}

func TestDefaultRestoreExecutor_Run_FatalError(t *testing.T) {
	ctrl := gomock.NewController(t)

	reader := bgmocks.NewMockStreamingReader(t)
	client := aerospike.NewMockClient(ctrl)
	client.EXPECT().
		Restore(gomock.Any(), gomock.Any(), reader).
		Return(nil, errors.New("restore failed"))

	operations := &mockStorageReader{reader: reader}
	executor := NewRestore(operations)

	_, err := executor.Run(t.Context(), client, testRestoreRequest())
	require.Error(t, err)
	assert.ErrorContains(t, err, "restore failed")
}

func TestDefaultRestoreExecutor_Run_AllEmptyStorage(t *testing.T) {
	operations := &mockStorageReader{err: common.ErrEmptyStorage}
	executor := NewRestore(operations)

	client := aerospike.NewMockClient(gomock.NewController(t))

	_, err := executor.Run(t.Context(), client, testRestoreRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, common.ErrEmptyStorage)
	assert.Equal(t, 2, operations.calls)
}

func TestRunScanRestore_CreateReaderError(t *testing.T) {
	client := aerospike.NewMockClient(gomock.NewController(t))
	operations := &mockStorageReader{err: errors.New("reader failed")}

	_, err := runScanRestore(t.Context(), client, testRestoreRequest(), operations)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to create backup reader")
}

func TestRunScanRestore_RestoreError(t *testing.T) {
	ctrl := gomock.NewController(t)

	reader := bgmocks.NewMockStreamingReader(t)
	client := aerospike.NewMockClient(ctrl)
	client.EXPECT().
		Restore(gomock.Any(), gomock.Any(), reader).
		Return(nil, errors.New("restore failed"))

	operations := &mockStorageReader{reader: reader}

	_, err := runScanRestore(t.Context(), client, testRestoreRequest(), operations)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to start restore")
}

func testRestoreRequest() *model.RestoreRequest {
	source := "source-ns"
	dest := "dest-ns"
	return &model.RestoreRequest{
		Policy: model.RestorePolicy{
			Namespace: &model.RestoreNamespace{
				Source:      &source,
				Destination: &dest,
			},
		},
		SourceStorage: &model.LocalStorage{Path: "/tmp/backups"},
	}
}
