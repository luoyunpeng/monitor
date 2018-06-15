package container

import (
	"fmt"
	"io"
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

type stats struct {
	mu sync.Mutex
	cs []*CStats
}

func (s *stats) add(cs *CStats) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.isKnownContainer(cs.ContainerID); !exists {
		s.cs = append(s.cs, cs)
		return true
	}
	return false
}

func (s *stats) remove(id string) {
	s.mu.Lock()
	if i, exists := s.isKnownContainer(id); exists {
		s.cs = append(s.cs[:i], s.cs[i+1:]...)
	}
	s.mu.Unlock()
}

func (s *stats) isKnownContainer(cid string) (int, bool) {
	for i, c := range s.cs {
		if c.ContainerID == cid || c.Name == cid {
			return i, true
		}
	}
	return -1, false
}

// ContainerStats represents an entity to store containers statistics synchronously
type CStats struct {
	mutex sync.Mutex
	HumanizeStats
	err error
}

// GetError returns the container statistics error.
// This is used to determine whether the statistics are valid or not
func (cs *CStats) GetError() error {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	return cs.err
}

// SetErrorAndReset zeroes all the container statistics and store the error.
// It is used when receiving time out error during statistics collecting to reduce lock overhead
func (cs *CStats) SetErrorAndReset(err error) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	cs.CPUPercentage = 0
	cs.Memory = 0
	cs.MemoryPercentage = 0
	cs.MemoryLimit = 0
	cs.NetworkRx = 0
	cs.NetworkTx = 0
	cs.BlockRead = 0
	cs.BlockWrite = 0
	cs.PidsCurrent = 0
	cs.err = err
	cs.IsInvalid = true
}

// SetError sets container statistics error
func (cs *CStats) SetError(err error) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	cs.err = err
	if err != nil {
		cs.IsInvalid = true
	}
}

// SetStatistics set the container statistics
func (cs *CStats) SetStatistics(s HumanizeStats) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	s.ContainerID = cs.ContainerID
	cs.HumanizeStats = s
}

// GetStatistics returns container statistics with other meta data such as the container name
func (cs *CStats) GetStatistics() *HumanizeStats {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	return &cs.HumanizeStats
}

// NewContainerStats returns a new ContainerStats entity and sets in it the given name
func NewContainerStats(containerID string) *CStats {
	return &CStats{HumanizeStats: HumanizeStats{ContainerID: containerID}}
}

type HumanizeStats struct {
	ContainerID      string
	Name             string
	CPUPercentage    float64
	Memory           float64 // On Windows this is the private working set
	MemoryLimit      float64 // Not used on Windows
	MemoryPercentage float64 // Not used on Windows
	NetworkRx        float64
	NetworkTx        float64
	BlockRead        float64
	BlockWrite       float64
	PidsCurrent      uint64 // Not used on Windows

	//time
	ReadTime    string
	PreReadTime string

	IsInvalid bool
}

// Deprecated: use Collect(respByte []byte)
func Set(response types.ContainerStats) (s *HumanizeStats) {
	var (
		previousCPU    uint64
		previousSystem uint64
	)

	dec := json.NewDecoder(response.Body)

	var (
		statsJSON              *types.StatsJSON
		memPercent, cpuPercent float64
		blkRead, blkWrite      uint64 // Only used on Linux
		mem, memLimit          float64
		pidsStatsCurrent       uint64
	)

	if err := dec.Decode(&statsJSON); err != nil {
		dec = json.NewDecoder(io.MultiReader(dec.Buffered(), response.Body))
		if err == io.EOF {
		}
		time.Sleep(100 * time.Millisecond)
	}

	previousCPU = statsJSON.PreCPUStats.CPUUsage.TotalUsage
	previousSystem = statsJSON.PreCPUStats.SystemUsage
	cpuPercent = CalculateCPUPercentUnix(previousCPU, previousSystem, statsJSON)
	blkRead, blkWrite = CalculateBlockIO(statsJSON.BlkioStats)
	mem = CalculateMemUsageUnixNoCache(statsJSON.MemoryStats)
	memLimit = float64(statsJSON.MemoryStats.Limit)
	memPercent = CalculateMemPercentUnixNoCache(memLimit, mem)
	pidsStatsCurrent = statsJSON.PidsStats.Current
	netRx, netTx := CalculateNetwork(statsJSON.Networks)

	//
	s.Name = statsJSON.Name
	s.ContainerID = statsJSON.ID
	s.CPUPercentage = cpuPercent
	fmt.Println(s.CPUPercentage)
	s.Memory = mem
	s.MemoryPercentage = memPercent
	s.MemoryLimit = memLimit
	s.NetworkRx = netRx
	s.NetworkTx = netTx
	s.BlockRead = float64(blkRead)
	s.BlockWrite = float64(blkWrite)
	s.PidsCurrent = pidsStatsCurrent
	return
}

func Collect(respByte []byte) (*HumanizeStats, error) {
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
	s := &HumanizeStats{}
	s.Name = statsJSON.Name[1:]
	s.ContainerID = statsJSON.ID
	s.CPUPercentage = math.Trunc(cpuPercent*1e2+0.5) * 1e-2
	s.Memory = mem
	s.MemoryPercentage = math.Trunc(memPercent*1e2+0.5) * 1e-2
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
	return cpuPercent
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
	return rx, tx
}

// calculateMemUsageUnixNoCache calculate memory usage of the container.
// Page cache is intentionally excluded to avoid misinterpretation of the output.
func CalculateMemUsageUnixNoCache(mem types.MemoryStats) float64 {
	return float64(mem.Usage - mem.Stats["cache"])
}

func CalculateMemPercentUnixNoCache(limit float64, usedNoCache float64) float64 {
	// MemoryStats.Limit will never be 0 unless the container is not running and we haven't
	// got any data from cgroup
	if limit != 0 {
		return usedNoCache / limit * 100.0
	}
	return 0
}
