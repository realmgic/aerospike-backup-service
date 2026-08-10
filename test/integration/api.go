//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerospike-backup-service/v3/pkg/dto"
	"github.com/aerospike/aerospike-backup-service/v3/pkg/model"
)

const (
	// backupTimeout bounds how long a test waits for an ad-hoc backup to appear.
	backupTimeout = 6 * time.Second
	// pollInterval is how often the service is asked for the current backup list.
	pollInterval = 250 * time.Millisecond
)

// fullBackupsURL is the ad-hoc full backup endpoint of the routine created by baseConfig.
func (e *env) fullBackupsURL() string {
	return fmt.Sprintf("%s/v1/backups/full/%s", e.server.URL, routineName)
}

// incrementalBackupsURL is the ad-hoc incremental backup endpoint of the routine created by baseConfig.
func (e *env) incrementalBackupsURL() string {
	return fmt.Sprintf("%s/v1/backups/incremental/%s", e.server.URL, routineName)
}

// triggerFullBackup asks the service to run a full backup now.
func (s *Suite) triggerFullBackup(e *env) {
	req, err := http.NewRequestWithContext(s.T().Context(), http.MethodPost, e.fullBackupsURL(), nil)
	s.Require().NoError(err)

	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		s.Require().Failf("failed to trigger full backup", "status %d: %s", resp.StatusCode, body)
	}
}

// triggerIncrementalBackup asks the service to run an incremental backup now.
func (s *Suite) triggerIncrementalBackup(e *env) {
	req, err := http.NewRequestWithContext(s.T().Context(), http.MethodPost, e.incrementalBackupsURL(), nil)
	s.Require().NoError(err)

	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		s.Require().Failf("failed to trigger incremental backup", "status %d: %s", resp.StatusCode, body)
	}
}

// waitForIncrementalBackup polls the routine until it reports the expected number of incremental backups.
func (s *Suite) waitForIncrementalBackup(e *env, want int) dto.BackupDetails {
	deadline := time.Now().Add(backupTimeout)

	for {
		backups := s.getIncrementalBackups(e)
		if len(backups) == want {
			return backups[want-1]
		}

		if time.Now().After(deadline) {
			s.Require().Failf("timed out waiting for incremental backup",
				"routine %q reported %d incremental backups after %s, want %d",
				routineName, len(backups), backupTimeout, want)
		}

		time.Sleep(pollInterval)
	}
}

// getIncrementalBackups returns the incremental backups the routine currently reports.
func (s *Suite) getIncrementalBackups(e *env) []dto.BackupDetails {
	req, err := http.NewRequestWithContext(s.T().Context(), http.MethodGet, e.incrementalBackupsURL(), nil)
	s.Require().NoError(err)

	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.Require().Failf("failed to fetch incremental backups", "status %d: %s", resp.StatusCode, body)
	}

	var backups []dto.BackupDetails
	s.Require().NoError(json.NewDecoder(resp.Body).Decode(&backups))

	return backups
}

// waitForFullBackup polls the routine until it reports exactly one full backup, and returns it.
func (s *Suite) waitForFullBackup(e *env) dto.BackupDetails {
	deadline := time.Now().Add(backupTimeout)

	for {
		backups := s.getFullBackups(e)
		if len(backups) == 1 {
			return backups[0]
		}

		if time.Now().After(deadline) {
			s.Require().Failf("timed out waiting for full backup",
				"routine %q reported %d full backups after %s, want 1",
				routineName, len(backups), backupTimeout)
		}

		time.Sleep(pollInterval)
	}
}

// getFullBackups returns the full backups the routine currently reports.
func (s *Suite) getFullBackups(e *env) []dto.BackupDetails {
	req, err := http.NewRequestWithContext(s.T().Context(), http.MethodGet, e.fullBackupsURL(), nil)
	s.Require().NoError(err)

	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.Require().Failf("failed to fetch full backups", "status %d: %s", resp.StatusCode, body)
	}

	var backups []dto.BackupDetails
	s.Require().NoError(json.NewDecoder(resp.Body).Decode(&backups))

	return backups
}

// restoreFullURL is the full-restore endpoint.
func (e *env) restoreFullURL() string {
	return fmt.Sprintf("%s/v1/restore/full", e.server.URL)
}

// restoreStatusURL is the status endpoint for a given restore job.
func (e *env) restoreStatusURL(jobID model.RestoreJobID) string {
	return fmt.Sprintf("%s/v1/restore/status/%d", e.server.URL, jobID)
}

// triggerRestore submits a full restore of backupDataPath into the suite's Aerospike cluster and
// returns the assigned job ID.
func (s *Suite) triggerRestore(e *env, backupDataPath string) model.RestoreJobID {
	body, err := json.Marshal(dto.RestoreRequest{
		DestinationClusterConfig: dto.DestinationClusterConfig{Name: clusterName},
		StorageConfig:            dto.StorageConfig{Name: storageName},
		Policy:                   &dto.RestorePolicy{},
		BackupDataPath:           backupDataPath,
	})
	s.Require().NoError(err)

	req, err := http.NewRequestWithContext(
		s.T().Context(), http.MethodPost, e.restoreFullURL(), bytes.NewReader(body),
	)
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)

	if resp.StatusCode != http.StatusAccepted {
		s.Require().Failf("failed to trigger restore", "status %d: %s", resp.StatusCode, respBody)
	}

	jobID, err := strconv.ParseInt(strings.TrimSpace(string(respBody)), 10, 64)
	s.Require().NoError(err)

	return model.RestoreJobID(jobID)
}

// waitForRestoreStatus polls the restore job until it leaves the running state, and returns its
// final status.
func (s *Suite) waitForRestoreStatus(e *env, jobID model.RestoreJobID) dto.RestoreJobStatus {
	deadline := time.Now().Add(backupTimeout)

	for {
		status := s.getRestoreStatus(e, jobID)
		if status.Status != dto.RestoreRunning {
			return status
		}

		if time.Now().After(deadline) {
			s.Require().Failf("timed out waiting for restore job to finish",
				"job %d still %q after %s", jobID, status.Status, backupTimeout)
		}

		time.Sleep(pollInterval)
	}
}

// getRestoreStatus fetches the current status of a restore job.
func (s *Suite) getRestoreStatus(e *env, jobID model.RestoreJobID) dto.RestoreJobStatus {
	req, err := http.NewRequestWithContext(s.T().Context(), http.MethodGet, e.restoreStatusURL(jobID), nil)
	s.Require().NoError(err)

	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.Require().Failf("failed to fetch restore status", "status %d: %s", resp.StatusCode, body)
	}

	var status dto.RestoreJobStatus
	s.Require().NoError(json.NewDecoder(resp.Body).Decode(&status))

	return status
}
