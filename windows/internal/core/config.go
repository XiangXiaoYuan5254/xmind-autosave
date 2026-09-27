// Package core holds the platform-independent parts of XMind Auto Save for
// Windows: configuration, per-file preferences, document path parsing and the
// save scheduler. Everything here is plain Go so it can be tested on any OS.
package core

import (
	"encoding/json"
	"time"
)

type Configuration struct {
	MonitoredFiles           []string `json:"monitoredFiles"`
	PollIntervalMilliseconds int      `json:"pollIntervalMilliseconds"`
	SaveDelayMilliseconds    int      `json:"saveDelayMilliseconds"`
	RetryMilliseconds        int      `json:"retryMilliseconds"`
	DirtyIndicators          []string `json:"dirtyIndicators"`
	ProcessNames             []string `json:"processNames"`
}

func DefaultConfiguration() Configuration {
	return Configuration{
		MonitoredFiles:           []string{},
		PollIntervalMilliseconds: 200,
		SaveDelayMilliseconds:    1_200,
		RetryMilliseconds:        2_000,
		DirtyIndicators:          []string{"已编辑", "Edited", "*"},
		ProcessNames:             []string{"Xmind.exe"},
	}
}

// ParseConfiguration overlays the JSON document on the defaults, so every key
// is optional.
func ParseConfiguration(data []byte) (Configuration, error) {
	configuration := DefaultConfiguration()
	if err := json.Unmarshal(data, &configuration); err != nil {
		return Configuration{}, err
	}
	return configuration, nil
}

func (c Configuration) PollInterval() time.Duration {
	return max(time.Duration(c.PollIntervalMilliseconds)*time.Millisecond, 100*time.Millisecond)
}

func (c Configuration) SaveDelay() time.Duration {
	return max(time.Duration(c.SaveDelayMilliseconds)*time.Millisecond, 0)
}

func (c Configuration) RetryInterval() time.Duration {
	return max(time.Duration(c.RetryMilliseconds)*time.Millisecond, 500*time.Millisecond)
}
