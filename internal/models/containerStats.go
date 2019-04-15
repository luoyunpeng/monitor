package models

import (
	"sync"
	"time"

	"github.com/luoyunpeng/monitor/internal/conf"
)

type ParsedConatinerMetric struct {
	CPUPercentage    float64
	Memory           float64
	MemoryLimit      float64
	MemoryPercentage float64
	NetworkRx        float64
	NetworkTx        float64
	BlockRead        float64
	BlockWrite       float64
	PidsCurrent      uint64

	//time
	ReadTime            string
	ReadTimeForInfluxDB time.Time
}

type ContainerStats struct {
	sync.RWMutex

	ID              string
	ContainerName   string
	ReadAbleMetrics []ParsedConatinerMetric

	isInvalid      bool
	isFirstCollect bool
}

// NewCMStack initial a NMStack point type
func NewCMetric(ContainerName, id string) *ContainerStats {
	return &ContainerStats{
		ContainerName:   ContainerName,
		ID:              id,
		ReadAbleMetrics: make([]ParsedConatinerMetric, 0, conf.DefaultReadLength),
		isFirstCollect:  true,
	}
}

func (cm *ContainerStats) Put(rdMetric ParsedConatinerMetric) bool {
	cm.Lock()

	if len(cm.ReadAbleMetrics) == conf.DefaultReadLength {
		//delete the first one also the oldest one, and append the latest one
		copy(cm.ReadAbleMetrics, cm.ReadAbleMetrics[1:])
		cm.ReadAbleMetrics[conf.DefaultReadLength-1] = rdMetric
		cm.Unlock()
		return true
	}
	cm.ReadAbleMetrics = append(cm.ReadAbleMetrics, rdMetric)
	cm.Unlock()
	return true
}

func (cm *ContainerStats) Read(num int) []ParsedConatinerMetric {
	cm.RLock()

	if len(cm.ReadAbleMetrics) == 0 {
		cm.RUnlock()
		return nil
	}
	var rdMetrics []ParsedConatinerMetric
	if len(cm.ReadAbleMetrics) >= num {
		rdMetrics = cm.ReadAbleMetrics[:num]
		cm.RUnlock()
		return rdMetrics
	}

	rdMetrics = cm.ReadAbleMetrics[:len(cm.ReadAbleMetrics)]
	cm.RUnlock()
	return rdMetrics
}

func (cm *ContainerStats) GetLatestMemory() float64 {
	cm.RLock()
	latestMem := cm.ReadAbleMetrics[len(cm.ReadAbleMetrics)-1].Memory
	cm.RUnlock()

	return latestMem
}

func (cm *ContainerStats) IsInValid() bool {
	return cm.isInvalid
}
