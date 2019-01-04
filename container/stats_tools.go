package container

import (
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/json-iterator/go"
	"github.com/luoyunpeng/monitor/common"
)

var (
	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

// DockerHost
type DockerHost struct {
	sync.RWMutex
	cms []*CMetric
	//indicate which host this stats belong to
	ip     string
	logger *log.Logger
	// Done close means that this host has been canceled for monitoring
	Done chan struct{}
}

// NewContainerHost
func NewDockerHost(ip string, logger *log.Logger) *DockerHost {
	return &DockerHost{ip: ip, logger: logger, Done: make(chan struct{})}
}

func (dh *DockerHost) Add(cm *CMetric) bool {
	dh.Lock()

	if dh.isKnownContainer(cm.ID) == -1 {
		dh.cms = append(dh.cms, cm)
		dh.Unlock()
		return true
	}
	dh.Unlock()
	return false
}

func (dh *DockerHost) Remove(id string) {
	dh.Lock()

	if i := dh.isKnownContainer(id); i != -1 {
		// set the container metric to invalid for stopping the collector, also remove container metrics stack
		dh.cms[i].isInvalid = true
		dh.cms = append(dh.cms[:i], dh.cms[i+1:]...)
	}
	dh.Unlock()
}

func (dh *DockerHost) StopCollect() {
	dh.Lock()

	//set all containerStack status to invalid, to stop all collecting
	for _, containerStack := range dh.cms {
		containerStack.isInvalid = true
	}
	close(dh.Done)
	dh.Unlock()
	dh.logger.Println("stop all container collect")
	StoppedDocker.Store(dh.ip, struct{}{})
}

//
func (dh *DockerHost) isKnownContainer(cid string) int {
	for i, cm := range dh.cms {
		if cm.ID == cid || cm.ContainerName == cid {
			return i
		}
	}
	return -1
}

func (dh *DockerHost) Length() int {
	dh.RLock()
	cmsLen := len(dh.cms)
	dh.RUnlock()

	return cmsLen
}

func (dh *DockerHost) AllNames() []string {
	dh.RLock()

	var names []string
	for _, cm := range dh.cms {
		names = append(names, cm.ContainerName)
	}
	dh.RUnlock()

	return names
}

func (dh *DockerHost) GetAllLastMemory() float64 {
	dh.RLock()

	var totalMem float64
	for _, cm := range dh.cms {
		totalMem += cm.GetLatestMemory()
	}
	dh.RUnlock()

	return totalMem
}

type CMetric struct {
	sync.RWMutex

	ID              string
	ContainerName   string
	ReadAbleMetrics []ParsedConatinerMetric

	isInvalid      bool
	isFirstCollect bool
}

// NewContainerMStack initial a NewContainerMStack point type
func NewCMetric(ContainerName, id string) *CMetric {
	return &CMetric{
		ContainerName:   ContainerName,
		ID:              id,
		ReadAbleMetrics: make([]ParsedConatinerMetric, 0, defaultReadLength),
		isFirstCollect:  true,
	}
}

func (cm *CMetric) Put(rdMetric ParsedConatinerMetric) bool {
	cm.Lock()

	if len(cm.ReadAbleMetrics) == defaultReadLength {
		//delete the first one also the oldest one, and append the latest one
		copy(cm.ReadAbleMetrics, cm.ReadAbleMetrics[1:])
		cm.ReadAbleMetrics[defaultReadLength-1] = rdMetric
		cm.Unlock()
		return true
	}
	cm.ReadAbleMetrics = append(cm.ReadAbleMetrics, rdMetric)
	cm.Unlock()
	return true
}

func (cm *CMetric) Read(num int) []ParsedConatinerMetric {
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

func (cm *CMetric) GetLatestMemory() float64 {
	cm.RLock()
	latestMem := cm.ReadAbleMetrics[len(cm.ReadAbleMetrics)-1].Memory
	cm.RUnlock()

	return latestMem
}

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

func IsKnownHost(host string) bool {
	for _, v := range common.HostIPs {
		if v == host {
			return true
		}
	}

	return false
}
