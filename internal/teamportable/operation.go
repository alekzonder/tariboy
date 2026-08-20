package teamportable

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type OperationStep struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	SourceReady bool   `json:"source_ready,omitempty"`
}

type Operation struct {
	ImportID       string            `json:"import_id"`
	Team           string            `json:"team"`
	Status         string            `json:"status"`
	Steps          []OperationStep   `json:"steps"`
	Error          string            `json:"error,omitempty"`
	Updated        string            `json:"updated_at"`
	ComposeYAML    string            `json:"compose_yaml,omitempty"`
	ResolvedRefs   map[string]string `json:"resolved_refs,omitempty"`
	UpdateExisting bool              `json:"update_existing,omitempty"`
}

func (s Service) leasePath(importID string) (string, error) {
	path, err := s.operationPath(importID)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), ".operation.lease"), nil
}

func (s Service) TouchLease(importID string) error {
	path, err := s.leasePath(importID)
	if err != nil {
		return err
	}
	now := time.Now()
	if err := os.Chtimes(path, now, now); err == nil {
		return nil
	}
	return os.WriteFile(path, nil, 0o600)
}

func (s Service) RemoveLease(importID string) {
	if path, err := s.leasePath(importID); err == nil {
		_ = os.Remove(path)
	}
}

func validImportID(importID string) bool {
	return importID != "" && filepath.Base(importID) == importID
}

func (s Service) operationPath(importID string) (string, error) {
	if !validImportID(importID) || s.StagingRoot == "" {
		return "", errors.New("team portable: invalid import id")
	}
	return filepath.Join(s.StagingRoot, importID, ".operation.json"), nil
}

func (s Service) SaveOperation(operation Operation) error {
	path, err := s.operationPath(operation.ImportID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	operation.Updated = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(operation)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".operation-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func (s Service) Operation(importID string) (Operation, error) {
	path, err := s.operationPath(importID)
	if err != nil {
		return Operation{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Operation{}, errors.New("team portable: operation not found")
	}
	var operation Operation
	if err := json.Unmarshal(data, &operation); err != nil {
		return Operation{}, err
	}
	if operation.ImportID != importID {
		return Operation{}, errors.New("team portable: operation id mismatch")
	}
	return operation, nil
}

func recoverStaleOperation(operation Operation, now time.Time, staleAfter time.Duration) Operation {
	if operation.Status != "running" {
		return operation
	}
	updated, err := time.Parse(time.RFC3339Nano, operation.Updated)
	if err == nil && now.Sub(updated) <= staleAfter {
		return operation
	}
	operation.Status = "failed"
	operation.Error = "import was interrupted; retry unfinished work"
	return operation
}

func (s Service) RecoverOperation(importID string, now time.Time, staleAfter time.Duration) (Operation, error) {
	operation, err := s.Operation(importID)
	if err != nil {
		return Operation{}, err
	}
	recovered := operation
	leaseFresh := false
	if path, pathErr := s.leasePath(importID); pathErr == nil {
		if info, statErr := os.Stat(path); statErr == nil {
			leaseFresh = now.Sub(info.ModTime()) <= staleAfter
		}
	}
	if !leaseFresh {
		recovered = recoverStaleOperation(operation, now, staleAfter)
	}
	if recovered.Status != operation.Status {
		if err := s.SaveOperation(recovered); err != nil {
			return Operation{}, err
		}
	}
	return recovered, nil
}
