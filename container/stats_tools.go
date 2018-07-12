package container

import (
	"math"
	"strings"
	"sync"
	"time"

	"github.com/json-iterator/go"

	"github.com/docker/docker/api/types"
)

var (
	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

//one docker host have only one hostContainerMStack
type hostContainerMStack struct {
	mu sync.RWMutex
	//indicate which host this stats belong to
	hostName string
	cms      []*containerMetricStack
}

func NewHostCMStack(host string) *hostContainerMStack {
	return &hostContainerMStack{hostName: host}
}

func (s *hostContainerMStack) add(newCms *containerMetricStack) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.isKnownContainer(newCms.ID); !exists {
		s.cms = append(s.cms, newCms)
		return true
	}
	return false
}

func (s *hostContainerMStack) remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if i, exists := s.isKnownContainer(id); exists {
		// set the container metric to invalid for stopping the collector, also rm container metrics stack
		s.cms[i].isInvalid = true
		s.cms = append(s.cms[:i], s.cms[i+1:]...)
	}
}

func (s *hostContainerMStack) isKnownContainer(cid string) (int, bool) {
	for i, c := range s.cms {
		if c.ID == cid || c.ContainerName == cid {
			return i, true
		}
	}
	return -1, false
}

func (s *hostContainerMStack) length() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.cms)
}

func (s *hostContainerMStack) allNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var names []string
	for _, cm := range s.cms {
		names = append(names, cm.ContainerName)
	}

	return names
}

type containerMetricStack struct {
	mu sync.RWMutex

	ID            string
	ContainerName string
	csFMetrics    []*ParsedConatinerMetrics

	isInvalid bool
}

func NewContainerMStack(ContainerName, id string) *containerMetricStack {
	return &containerMetricStack{ContainerName: ContainerName, ID: id}
}

func (cms *containerMetricStack) put(cfm *ParsedConatinerMetrics) bool {
	cms.mu.Lock()
	defer cms.mu.Unlock()

	if len(cms.csFMetrics) == 15 {
		//cms.csFMetrics = append(cms.csFMetrics, cfm)
		//delete the first one also the oldest one, and append the latest one
		cms.csFMetrics = append(cms.csFMetrics[1:], cfm)
		return true
	}
	cms.csFMetrics = append(cms.csFMetrics, cfm)
	return true
}

func (cms *containerMetricStack) read(num int) []*ParsedConatinerMetrics {
	cms.mu.RLock()
	defer cms.mu.RUnlock()

	if len(cms.csFMetrics) == 0 {
		return nil
	}
	if len(cms.csFMetrics) >= num {
		return cms.csFMetrics[:num]
	}
	return cms.csFMetrics[:len(cms.csFMetrics)]
}

func (cms *containerMetricStack) length() int {
	cms.mu.RLock()
	defer cms.mu.RUnlock()

	return len(cms.csFMetrics)
}

// NewContainerStats returns a new ContainerStats entity and sets in it the given name
func NewParsedConatinerMetrics() *ParsedConatinerMetrics {
	return &ParsedConatinerMetrics{}
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
	ReadTime    string
	PreReadTime string
}

func Parse(respByte []byte) (*ParsedConatinerMetrics, error) {
	var (
		previousCPU    uint64
		previousSystem uint64

		statsJSON              *types.StatsJSON
		memPercent, cpuPercent float64
		blkRead, blkWrite      uint64 // Only used on Linux
		mem, memLimit          float64
		pidsStatsCurrent       uint64
	)

	err := json.Unmarshal(respByte, &statsJSON)
	if err != nil {
		return nil, err
	}

	previousCPU = statsJSON.PreCPUStats.CPUUsage.TotalUsage
	previousSystem = statsJSON.PreCPUStats.SystemUsage
	cpuPercent = CalculateCPUPercentUnix(previousCPU, previousSystem, statsJSON)
	blkRead, blkWrite = CalculateBlockIO(statsJSON.BlkioStats)
	mem = CalculateMemUsageUnixNoCache(statsJSON.MemoryStats) / (1024 * 1024)
	memLimit = float64(statsJSON.MemoryStats.Limit) / (1024 * 1024)
	memPercent = CalculateMemPercentUnixNoCache(memLimit, mem)
	pidsStatsCurrent = statsJSON.PidsStats.Current
	netRx, netTx := CalculateNetwork(statsJSON.Networks)

	//
	s := &ParsedConatinerMetrics{}
	//s.Name = statsJSON.Name[1:]
	//s.ContainerID = statsJSON.ID
	s.CPUPercentage = cpuPercent
	s.Memory = mem
	s.MemoryPercentage = memPercent
	s.MemoryLimit = memLimit
	s.NetworkRx = netRx
	s.NetworkTx = netTx
	s.BlockRead = float64(blkRead)
	s.BlockWrite = float64(blkWrite)
	s.PidsCurrent = pidsStatsCurrent
	s.ReadTime = statsJSON.Read.Add(time.Hour * 8).Format("2006-01-02 15:04:05")
	s.PreReadTime = statsJSON.PreRead.Add(time.Hour * 8).Format("2006-01-02 15:04:05")
	return s, nil
}

func CalculateCPUPercentUnix(previousCPU, previousSystem uint64, v *types.StatsJSON) float64 {
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

//return given the significant digit of float64
func Round(f float64, n int) float64 {
	pow10N := math.Pow10(n)
	return math.Trunc((f+0.5/pow10N)*pow10N) / pow10N
}
