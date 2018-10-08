package container

import (
	"math"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/json-iterator/go"
)

var (
	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

//one docker host have only one hostContainerMStack
type HostContainerMetricStack struct {
	sync.RWMutex
	//indicate which host this stats belong to
	hostName string
	cms      []*SingalContainerMetricStack
}

// NewHostContainerMetricStack initial a HostContainerMetricStack point type
func NewHostContainerMetricStack(host string) *HostContainerMetricStack {
	return &HostContainerMetricStack{hostName: host}
}

func (s *HostContainerMetricStack) Add(newCms *SingalContainerMetricStack) bool {
	s.Lock()
	defer s.Unlock()

	if _, exists := s.isKnownContainer(newCms.ID); !exists {
		s.cms = append(s.cms, newCms)
		return true
	}
	return false
}

func (s *HostContainerMetricStack) Remove(id string) {
	s.Lock()
	defer s.Unlock()

	if i, exists := s.isKnownContainer(id); exists {
		// set the container metric to invalid for stopping the collector, also remove container metrics stack
		s.cms[i].isInvalid = true
		s.cms = append(s.cms[:i], s.cms[i+1:]...)
	}
}

func (s *HostContainerMetricStack) isKnownContainer(cid string) (int, bool) {
	for i, c := range s.cms {
		if c.ID == cid || c.ContainerName == cid {
			return i, true
		}
	}
	return -1, false
}

func (s *HostContainerMetricStack) Length() int {
	s.RLock()
	defer s.RUnlock()

	return len(s.cms)
}

func (s *HostContainerMetricStack) AllNames() []string {
	s.RLock()
	defer s.RUnlock()

	var names []string
	for _, cm := range s.cms {
		names = append(names, cm.ContainerName)
	}

	return names
}

func (s *HostContainerMetricStack) GetAllLastMemory() float64 {
	s.RLock()
	defer s.RUnlock()

	var totalMem float64
	for _, cm := range s.cms {
		totalMem += cm.GetLatestMemory()
	}

	return totalMem
}

type SingalContainerMetricStack struct {
	mu sync.RWMutex

	ID              string
	ContainerName   string
	ReadAbleMetrics []ParsedConatinerMetrics

	isInvalid      bool
	isFirstCollect bool
}

// NewContainerMStack initial a NewContainerMStack point type
func NewContainerMStack(ContainerName, id string) *SingalContainerMetricStack {
	return &SingalContainerMetricStack{
		ContainerName:   ContainerName,
		ID:              id,
		ReadAbleMetrics: make([]ParsedConatinerMetrics, 0, defaultReadLength),
		isFirstCollect:  true,
	}
}

func (cms *SingalContainerMetricStack) Put(cfm ParsedConatinerMetrics) bool {
	cms.mu.Lock()
	defer cms.mu.Unlock()

	if len(cms.ReadAbleMetrics) == defaultReadLength {
		//delete the first one also the oldest one, and append the latest one
		copy(cms.ReadAbleMetrics, cms.ReadAbleMetrics[1:])
		cms.ReadAbleMetrics[defaultReadLength-1] = cfm
		return true
	}
	cms.ReadAbleMetrics = append(cms.ReadAbleMetrics, cfm)
	return true
}

func (cms *SingalContainerMetricStack) Read(num int) []ParsedConatinerMetrics {
	cms.mu.RLock()
	defer cms.mu.RUnlock()

	if len(cms.ReadAbleMetrics) == 0 {
		return nil
	}
	if len(cms.ReadAbleMetrics) >= num {
		return cms.ReadAbleMetrics[:num]
	}
	return cms.ReadAbleMetrics[:len(cms.ReadAbleMetrics)]
}

func (cms *SingalContainerMetricStack) GetLatestMemory() float64 {
	cms.mu.RLock()
	defer cms.mu.RUnlock()

	return cms.ReadAbleMetrics[len(cms.ReadAbleMetrics)-1].Memory
}

type ParsedConatinerMetrics struct {
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
	return Round(cpuPercent, 6)
}

func CalculateBlockIO(blkio types.BlkioStats) (uint64, uint64) {
	var blkRead, blkWrite uint64
	for _, bioEntry := range blkio.IoServiceBytesRecursive {
		switch strings.ToLower(bioEntry.Op) {
		case "read":
			blkRead = blkRead + bioEntry.Value
		case "write":
			blkWrite = blkWrite + bioEntry.Value
		}
	}
	return blkRead, blkWrite
}

func CalculateNetwork(network map[string]types.NetworkStats) (float64, float64) {
	var rx, tx float64

	for _, v := range network {
		rx += float64(v.RxBytes)
		tx += float64(v.TxBytes)
	}
	return Round(rx/(1024*1024), 3), Round(tx/(1024*1024), 3)
}

// calculateMemUsageUnixNoCache calculate memory usage of the container.
// Page cache is intentionally excluded to avoid misinterpretation of the output.
func CalculateMemUsageUnixNoCache(mem types.MemoryStats) float64 {
	return Round(float64(mem.Usage-mem.Stats["cache"])/(1024*1024), 2)
}

func CalculateMemPercentUnixNoCache(limit float64, usedNoCache float64) float64 {
	// MemoryStats.Limit will never be 0 unless the container is not running and we haven't
	// got any data from cGroup
	if limit != 0 {
		return Round(usedNoCache/limit*100.0, 3)
	}
	return 0
}

// Round return given the significant digit of float64
func Round(f float64, n int) float64 {
	pow10N := math.Pow10(n)
	return math.Trunc((f+0.5/pow10N)*pow10N) / pow10N
}
