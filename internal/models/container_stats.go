package models

import (
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/luoyunpeng/monitor/internal/config"
	"github.com/luoyunpeng/monitor/internal/util"
)

// ParsedConatinerMetric is parsed message, easy to understand
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

	// ReadTime for rest api all
	ReadTime string
	//
	ReadSec     int64
	ReadNanoSec int
	//ReadTimeForInfluxDB time.Time
}

// ContainerStats is container metric cache
type ContainerStats struct {
	sync.RWMutex

	ID              string
	ContainerName   string
	ReadAbleMetrics []ParsedConatinerMetric

	isInvalid      bool
	isFirstCollect bool
}

// NewCMetric initial a ContainerStats
func NewCMetric(ContainerName, id string) *ContainerStats {
	return &ContainerStats{
		ContainerName:   ContainerName,
		ID:              id,
		ReadAbleMetrics: make([]ParsedConatinerMetric, 0, config.MonitorInfo.CacheNum),
		isFirstCollect:  true,
	}
}

// Put add container metric to cache
func (cm *ContainerStats) Put(rdMetric ParsedConatinerMetric) bool {
	cm.Lock()

	if len(cm.ReadAbleMetrics) == config.MonitorInfo.CacheNum {
		// delete the first one also the oldest one, and append the latest one
		copy(cm.ReadAbleMetrics, cm.ReadAbleMetrics[1:])
		cm.ReadAbleMetrics[config.MonitorInfo.CacheNum-1] = rdMetric
		cm.Unlock()
		return true
	}
	cm.ReadAbleMetrics = append(cm.ReadAbleMetrics, rdMetric)
	cm.Unlock()
	return true
}

// Read get the container metric from cache
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

// GetLatestMemory get the latest memory info
func (cm *ContainerStats) GetLatestMemory() float64 {
	cm.RLock()
	defer cm.RUnlock()

	if len(cm.ReadAbleMetrics) >= 1 {
		return cm.ReadAbleMetrics[len(cm.ReadAbleMetrics)-1].Memory
	}

	return 0
}

// IsInValid check if current container is in monitoring
func (cm *ContainerStats) IsInValid() bool {
	return cm.isInvalid
}

// CalculateCPUPercentUnix calculate cpu used percentage
func CalculateCPUPercentUnix(previousCPU, previousSystem uint64, v types.StatsJSON) float64 {
	var (
		cpuPercent = 0.0
		// calculate the change for the cpu usage of the container in between readings
		cpuDelta = float64(v.CPUStats.CPUUsage.TotalUsage) - float64(previousCPU)
		// calculate the change for the entire system between readings
		systemDelta = float64(v.CPUStats.SystemUsage) - float64(previousSystem)
		onlineCPUs  = float64(v.CPUStats.OnlineCPUs)
	)

	if onlineCPUs == 0.0 {
		onlineCPUs = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
	}
	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
	}
	return util.Round(cpuPercent, 6)
}

// CalculateBlockIO calculate block io
func CalculateBlockIO(blkio types.BlkioStats) (uint64, uint64) {
	var blkRead, blkWrite uint64
	for _, bioEntry := range blkio.IoServiceBytesRecursive {
		if len(bioEntry.Op) == 0 {
			continue
		}
		switch bioEntry.Op[0] {
		case 'r', 'R':
			blkRead = blkRead + bioEntry.Value
		case 'w', 'W':
			blkWrite = blkWrite + bioEntry.Value
		}
	}
	return blkRead, blkWrite
}

// CalculateNetwork calculate network io
func CalculateNetwork(network map[string]types.NetworkStats) (float64, float64) {
	var rx, tx float64

	for _, v := range network {
		rx += float64(v.RxBytes)
		tx += float64(v.TxBytes)
	}
	return util.Round(rx/(1024*1024), 3), util.Round(tx/(1024*1024), 3)
}

// CalculateMemUsageUnixNoCache calculate memory usage of the container.
// Page cache is intentionally excluded to avoid misinterpretation of the output.
func CalculateMemUsageUnixNoCache(mem types.MemoryStats) float64 {
	return util.Round(float64(mem.Usage-mem.Stats["cache"])/(1024*1024), 2)
}

// CalculateMemPercentUnixNoCache calculate memory usage percentage of the container.
func CalculateMemPercentUnixNoCache(limit float64, usedNoCache float64) float64 {
	// MemoryStats.Limit will never be 0 unless the container is not running and we haven't
	// got any data from cGroup
	if limit != 0 {
		return util.Round(usedNoCache/limit*100.0, 3)
	}
	return 0
}
