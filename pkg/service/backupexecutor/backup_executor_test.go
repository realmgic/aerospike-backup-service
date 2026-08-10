package backupexecutor

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/aerospike/aerospike-backup-service/v3/pkg/model"
	"github.com/aerospike/aerospike-backup-service/v3/pkg/service/aerospike"
	"github.com/aerospike/aerospike-backup-service/v3/pkg/util/ptr"
	"github.com/aerospike/backup-go"
	"github.com/aerospike/backup-go/io/storage/options"
	bgmocks "github.com/aerospike/backup-go/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type mockStorageWriter struct {
	writer backup.Writer
	err    error
}

func (m *mockStorageWriter) CreateDirWriter(
	_ context.Context,
	_ model.Storage,
	_ string,
	_ ...options.Opt,
) (backup.Writer, error) {
	return m.writer, m.err
}

func TestIsFullBackup(t *testing.T) {
	assert.True(t, isFullBackup(model.TimeBounds{}))
	assert.True(t, isFullBackup(model.TimeBounds{FromTime: nil}))
	assert.False(t, isFullBackup(model.TimeBounds{FromTime: ptr.Of(time.Now())}))
}

func TestDefaultBackupExecutor_Run_GetClientError(t *testing.T) {
	ctrl := gomock.NewController(t)

	clientManager := aerospike.NewMockClientManager(ctrl)
	clientManager.EXPECT().
		GetClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("client unavailable"))

	executor := NewDefaultBackupExecutor(clientManager, &mockStorageWriter{})
	routine := testBackupRoutine()

	_, err := executor.Run(
		t.Context(),
		routine,
		model.TimeBounds{},
		"test-ns",
		"/backup/path",
		nil,
		slog.Default(),
	)

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to get backup client")
}

func TestDefaultBackupExecutor_Run_CreateWriterError(t *testing.T) {
	ctrl := gomock.NewController(t)

	client := aerospike.NewMockClient(ctrl)
	clientManager := aerospike.NewMockClientManager(ctrl)
	clientManager.EXPECT().
		GetClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(client, nil)
	clientManager.EXPECT().Close(client)

	executor := NewDefaultBackupExecutor(clientManager, &mockStorageWriter{
		err: errors.New("writer failed"),
	})

	_, err := executor.Run(
		t.Context(),
		testBackupRoutine(),
		model.TimeBounds{},
		"test-ns",
		"/backup/path",
		nil,
		slog.Default(),
	)

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to create backup writer")
}

func TestDefaultBackupExecutor_Run_ScanBackupUsesScanPath(t *testing.T) {
	ctrl := gomock.NewController(t)

	writer := bgmocks.NewMockWriter(t)
	client := aerospike.NewMockClient(ctrl)
	clientManager := aerospike.NewMockClientManager(ctrl)

	clientManager.EXPECT().
		GetClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(client, nil)
	clientManager.EXPECT().Close(client)

	client.EXPECT().
		Backup(gomock.Any(), gomock.Any(), writer, gomock.Nil()).
		Return(nil, errors.New("backup start failed"))

	executor := NewDefaultBackupExecutor(clientManager, &mockStorageWriter{writer: writer})

	_, err := executor.Run(
		t.Context(),
		testBackupRoutine(),
		model.TimeBounds{},
		"test-ns",
		"/backup/path",
		nil,
		slog.Default(),
	)

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to start scan backup")
}

func TestRunScanBackup_ConfigError(t *testing.T) {
	client := aerospike.NewMockClient(gomock.NewController(t))

	_, err := runScanBackup(
		t.Context(),
		client,
		&model.BackupRoutine{
			BackupPolicy:     &model.BackupPolicy{},
			SourceCluster:    &model.AerospikeCluster{},
			FilterExpression: "invalid-expression",
		},
		model.TimeBounds{},
		"test-ns",
		nil,
	)

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to make backup config")
}

func TestRunScanBackup_BackupError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := aerospike.NewMockClient(ctrl)

	client.EXPECT().
		Backup(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Nil()).
		Return(nil, errors.New("backup failed"))

	_, err := runScanBackup(
		t.Context(),
		client,
		testBackupRoutine(),
		model.TimeBounds{},
		"test-ns",
		nil,
	)

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to start scan backup")
}

func testBackupRoutine() *model.BackupRoutine {
	return &model.BackupRoutine{
		BackupPolicy:  &model.BackupPolicy{},
		IntervalCron:  "@daily",
		SourceCluster: &model.AerospikeCluster{},
		Storage:       &model.LocalStorage{Path: "/tmp/backups"},
	}
}
