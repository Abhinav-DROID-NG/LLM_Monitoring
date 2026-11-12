package groundtruth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Layers struct {
	Model         bool `json:"model"`
	Safety        bool `json:"safety"`
	RequestConfig bool `json:"request_config"`
	Serving       bool `json:"serving"`
}

type Entry struct {
	TimestampUTC time.Time `json:"timestamp_utc"`
	ScenarioID   int       `json:"scenario_id"`
	TrueLayers   Layers    `json:"true_layers"`
}

type Log struct {
	mu         sync.Mutex
	entries    []Entry
	schedule   []Entry
	commitment string
	sealed     bool
}

func New(schedule []Entry) *Log {
	return &Log{schedule: append([]Entry(nil), schedule...), sealed: true}
}

func (l *Log) Commit() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.sealed {
		return "", errors.New("ground truth has already been unsealed")
	}
	data, err := json.Marshal(l.schedule)
	if err != nil {
		return "", fmt.Errorf("marshal ground-truth schedule: %w", err)
	}
	sum := sha256.Sum256(data)
	l.commitment = hex.EncodeToString(sum[:])
	return l.commitment, nil
}

func (l *Log) Append(entry Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.sealed {
		return errors.New("ground truth is no longer sealed")
	}
	entry.TimestampUTC = entry.TimestampUTC.UTC()
	l.entries = append(l.entries, entry)
	return nil
}

func (l *Log) Commitment() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.commitment
}

func (l *Log) Unseal(commitment string) ([]Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.commitment == "" || commitment != l.commitment {
		return nil, errors.New("ground-truth commitment verification failed")
	}
	if !l.sealed {
		return nil, errors.New("ground truth is already unsealed")
	}
	l.sealed = false
	return append([]Entry(nil), l.entries...), nil
}
