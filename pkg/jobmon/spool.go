package jobmon

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nightlyone/lockfile"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/securez-one/cagent/pkg/common"
)

const (
	markerRunning         = "0"
	markerFinished        = "1"
	spoolDirPermissions   = 6777
	spoolEntryPermissions = 0666
	jsonExtension         = "json"
)

var ErrJobAlreadyRunning = errors.New("A job with same ID is already running")

type SpoolManager struct {
	dirPath string
	logger  *logrus.Logger
}

// NewSpoolManager creates a new object to manage jobmon spool
// dirPath must be absolute path
func NewSpoolManager(dirPath string, logger *logrus.Logger) *SpoolManager {
	return &SpoolManager{dirPath, logger}
}

func (s *SpoolManager) NewJob(r *JobRun, forcedRun bool) (string, error) {
	l, err := s.getLock(r.ID)
	if err != nil {
		return "", err
	}
	defer s.releaseLock(l)

	duplicateRunEntries, err := s.findDuplicateRuns(r.ID)
	if err != nil {
		return "", err
	}
	alreadyRunning := len(duplicateRunEntries) > 0
	if !forcedRun && alreadyRunning {
		r.AddError(ErrJobAlreadyRunning.Error())
	}

	if forcedRun {
		err = removeFiles(duplicateRunEntries)
		if err != nil {
			return "", err
		}
	}

	uniqID := getUniqJobRunID(r.ID, alreadyRunning, r.StartedAt)
	filePath := s.getFilePath(uniqID)
	err = s.saveJobRun(filePath, r)
	if err != nil {
		return "", err
	}

	if !forcedRun && alreadyRunning {
		err = ErrJobAlreadyRunning
	}

	return uniqID, err
}

func (s *SpoolManager) FinishJob(uniqID string, r *JobRun) error {
	l, err := s.getLock(r.ID)
	if err != nil {
		return err
	}
	defer s.releaseLock(l)

	filePath := s.getFilePath(uniqID)
	newFilePath := s.getFilePath(getUniqJobRunID(r.ID, true, r.StartedAt))
	err = os.Rename(filePath, newFilePath)
	if err != nil {
		return errors.Wrapf(err, "could not mark job %s (unique %s) as finished", r.ID, uniqID)
	}

	return s.saveJobRun(newFilePath, r)
}

func (s *SpoolManager) GetFinishedJobs() ([]string, []*JobRun, error) {
	pattern := fmt.Sprintf("%s/%s_*_*.%s", s.dirPath, markerFinished, jsonExtension)
	fileNames, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, err
	}

	ids := make([]string, 0)
	jobs := make([]*JobRun, 0)
	for _, f := range fileNames {
		j, err := s.readEntryFile(f)
		if err != nil {
			return nil, nil, err
		}
		ids = append(ids, getUniqJobRunID(j.ID, true, j.StartedAt))
		jobs = append(jobs, j)
	}

	return ids, jobs, nil
}

func (s *SpoolManager) readEntryFile(path string) (*JobRun, error) {
	jsonFile, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "while opening file %s", path)
	}
	defer jsonFile.Close()
	var j JobRun
	err = json.NewDecoder(jsonFile).Decode(&j)
	if err != nil {
		return nil, errors.Wrapf(err, "while decoding file %s", path)
	}
	return &j, nil
}

func (s *SpoolManager) RemoveJobs(ids []string) error {
	var filePaths []string
	for _, uniqID := range ids {
		filePaths = append(filePaths, s.getFilePath(uniqID))
	}
	return removeFiles(filePaths)
}

func (s *SpoolManager) ensureSpoolDirExists() error {
	_, err := os.Stat(s.dirPath)
	if os.IsNotExist(err) {
		err = os.MkdirAll(s.dirPath, spoolDirPermissions)
		if err != nil {
			return errors.Wrapf(
				err,
				"could not create spool dir %s. Please check you have enough rights or try create the dir manually",
				s.dirPath,
			)
		}
	} else if err != nil {
		err = errors.Wrapf(err, "while checking spool dir %s exists", s.dirPath)
	}
	return err
}

func (s *SpoolManager) saveJobRun(filePath string, r *JobRun) error {
	err := s.ensureSpoolDirExists()
	if err != nil {
		return err
	}

	fl, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, spoolEntryPermissions)
	if err != nil {
		return errors.Wrapf(err, "can not open file for writing: %s", filePath)
	}
	defer fl.Close()

	err = json.NewEncoder(fl).Encode(r)
	if err != nil {
		return errors.Wrapf(err, "while encoding spool entry to %s", filePath)
	}
	return nil
}

func (s *SpoolManager) findDuplicateRuns(jobID string) ([]string, error) {
	encodedJobID := encodeJobID(jobID)
	pattern := fmt.Sprintf("%s/%s_*_%s.%s", s.dirPath, markerRunning, encodedJobID, jsonExtension)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, errors.Wrapf(err, "job %s failed while searching %s", jobID, pattern)
	}
	return matches, nil
}

func (s *SpoolManager) getLock(jobID string) (*lockfile.Lockfile, error) {
	err := s.ensureSpoolDirExists()
	if err != nil {
		return nil, err
	}

	l, err := lockfile.New(fmt.Sprintf("%s/job_%s.lock", s.dirPath, encodeJobID(jobID)))
	if err != nil {
		return nil, err
	}

	retryLimit := 20
	retry := 0
	retryIn := 500 * time.Millisecond
	for {
		err = l.TryLock()
		if err != nil {
			retry++
			if retry >= retryLimit {
				break
			}
			ownerProc, getOwnerErr := l.GetOwner()
			ownerInfo := "<unknown>"
			if getOwnerErr != nil {
				ownerInfo = fmt.Sprintf("could not get lock owner info: %s", getOwnerErr)
			} else if ownerProc != nil {
				ownerInfo = fmt.Sprintf("process with id %d", ownerProc.Pid)
			}
			s.logger.Errorf("job %s: could not get lock. Lockfile owner: %s. Attempt %d of %d, retrying in %v", jobID, ownerInfo, retry, retryLimit, retryIn)
			time.Sleep(retryIn)
		} else {
			return &l, nil
		}
	}

	return nil, err
}

func (s *SpoolManager) releaseLock(l *lockfile.Lockfile) {
	err := l.Unlock()
	if err != nil {
		s.logger.WithError(err).Error("could not release lock")
	}
}

func (s *SpoolManager) getFilePath(id string) string {
	return fmt.Sprintf("%s/%s.%s", s.dirPath, id, jsonExtension)
}

func getUniqJobRunID(jobID string, isJobFinished bool, jobStartedAt common.Timestamp) string {
	marker := markerRunning
	if isJobFinished {
		marker = markerFinished
	}
	parts := []string{
		marker,
		strconv.FormatInt(time.Time(jobStartedAt).Unix(), 10),
		encodeJobID(jobID),
	}
	return strings.Join(parts, "_")
}

// encodeJobID returns hex-encoded string for specified value.
// Max result length is len(id)*2.
func encodeJobID(id string) string {
	return hex.EncodeToString([]byte(id))
}

func removeFiles(filePaths []string) error {
	for _, f := range filePaths {
		err := removeFile(f)
		if err != nil {
			return err
		}
	}
	return nil
}

// removeFile ignores error if file already deleted or not exists
func removeFile(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "while removing %s", path)
	}
	return nil
}
