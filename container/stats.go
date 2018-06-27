package container

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

var (
	allHostStack []*hostContainerMStack
	mu           sync.RWMutex
)

func initLog(ip string) *log.Logger {
	file, err := os.OpenFile(ip+".cmonitor", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("Failed to open error log file:", err)
	}

	return log.New(file, "", log.Ldate|log.Ltime)
}

func KeepStats(dockerCli *client.Client, ip string) {
	closeChan := make(chan error)

	ctx := context.Background()
	// monitorContainerEvents watches for container creation and removal (only
	// used when calling `docker stats` without arguments).
	monitorContainerEvents := func(started chan<- struct{}, c chan events.Message) {
		f := filters.NewArgs()
		f.Add("type", "container")
		options := types.EventsOptions{
			Filters: f,
		}

		eventq, errq := dockerCli.Events(ctx, options)

		// Whether we successfully subscribed to eventq or not, we can now
		// unblock the main goroutine.
		close(started)

		// wait for events happens
		for {
			select {
			case event := <-eventq:
				c <- event
			case err := <-errq:
				closeChan <- err
				return
			}
		}
	}

	// waitFirst is a WaitGroup to wait first stat data's reach for each container
	waitFirst := &sync.WaitGroup{}

	// getContainerList simulates creation event for all previously existing
	// containers (only used when calling `docker stats` without arguments).
	logger := initLog(ip)
	hcmsStack := NewHostCMStack(ip)
	addToAllHostStack(hcmsStack)
	//allHostStack = append(allHostStack, hcmsStack)
	logger.Println("ID  NAME  CPU %  MEM  USAGE / LIMIT  MEM %  NET I/O  BLOCK I/O ")
	getContainerList := func() {
		options := types.ContainerListOptions{
			All: false,
		}
		cs, err := dockerCli.ContainerList(ctx, options)
		if err != nil {
			closeChan <- err
		}
		for _, container := range cs {
			cms := NewContainerMStack("", container.ID[:12])
			if hcmsStack.add(cms) {
				waitFirst.Add(1)
				go collect(ctx, cms, dockerCli, waitFirst, logger)
			}
		}
	}

	// If no names were specified, start a long running goroutine which
	// monitors container events. We make sure we're subscribed before
	// retrieving the list of running containers to avoid a race where we
	// would "miss" a creation.
	started := make(chan struct{})
	eh := InitEventHandler()
	eh.Handle("start", func(e events.Message) {
		logger.Println("event handler: received start event: %v", e)
		cms := NewContainerMStack("", e.ID[:12])
		if hcmsStack.add(cms) {
			waitFirst.Add(1)
			go collect(ctx, cms, dockerCli, waitFirst, logger)
		}
	})

	eh.Handle("die", func(e events.Message) {
		logger.Println("event handler: received die event: %v", e)
		hcmsStack.remove(e.ID[:12])
	})

	eventChan := make(chan events.Message)
	go eh.Watch(eventChan)
	go monitorContainerEvents(started, eventChan)
	defer func() {
		close(eventChan)
		close(closeChan)
		dockerCli.Close()
	}()
	// wait event listener go routine started
	<-started

	// Start a short-lived goroutine to retrieve the initial list of
	// containers.
	getContainerList()
	waitFirst.Wait()

	for {
		select {
		case err, ok := <-closeChan:
			if ok {
				if err != nil {
					// this is suppressing "unexpected EOF" in the cli when the
					// daemon restarts so it shutdowns cleanly
					if err == io.ErrUnexpectedEOF {

					}
					logger.Printf("err when keeping monitor : %v ", err)
					return
				}
			}
		default:
			time.Sleep(15 * time.Second)
		}
	}
}

func collect(ctx context.Context, cms *containerMetricStack, cli *client.Client, waitFirst *sync.WaitGroup, logger *log.Logger) {
	var (
		getFirst   bool
		u          = make(chan error, 1)
		errNoSuchC = errors.New("no such container")
		cfm        *ContainerFMetrics
	)

	defer func() {
		// if error happens and we get nothing of stats, release wait group whatever
		if !getFirst {
			getFirst = true
			waitFirst.Done()
		}
	}()

	go func() {
		for {
			var (
				previousCPU    uint64
				previousSystem uint64

				statsJSON              *types.StatsJSON
				memPercent, cpuPercent float64
				blkRead, blkWrite      uint64 // Only used on Linux
				mem, memLimit          float64
				pidsStatsCurrent       uint64
			)

			response, err := cli.ContainerStats(ctx, cms.id, false)
			if err != nil {
				logger.Printf("collecting stats for %v", err)
				if strings.Contains(err.Error(), "No such container") {
					u <- errNoSuchC
				}
				return
			}

			// bool value initial value is false
			if cms.isInvalid {
				logger.Println(cms.name, " is not running, stop collecting in goroutine")
				response.Body.Close()
				u <- errNoSuchC
				return
			}

			respByte, err := ioutil.ReadAll(response.Body)
			if err != nil {
				logger.Printf("collecting stats for %v", err)
				return
			}

			errUnmarshal := json.Unmarshal(respByte, &statsJSON)
			if errUnmarshal != nil {
				logger.Printf("Unmarshal collecting stats for %v", errUnmarshal)
				return
			}

			previousCPU = statsJSON.PreCPUStats.CPUUsage.TotalUsage
			previousSystem = statsJSON.PreCPUStats.SystemUsage
			cpuPercent = CalculateCPUPercentUnix(previousCPU, previousSystem, statsJSON)
			blkRead, blkWrite = CalculateBlockIO(statsJSON.BlkioStats)
			// change mem related metric to MB
			mem = CalculateMemUsageUnixNoCache(statsJSON.MemoryStats) / (1024 * 1024)
			memLimit = float64(statsJSON.MemoryStats.Limit) / (1024 * 1024)
			memPercent = CalculateMemPercentUnixNoCache(memLimit, mem)
			pidsStatsCurrent = statsJSON.PidsStats.Current
			netRx, netTx := CalculateNetwork(statsJSON.Networks)

			cfm = NewContainerStats(cms.id)
			cfm.Name = statsJSON.Name[1:]
			if cms.name == "" {
				cms.name = cfm.Name
			}
			cfm.CPUPercentage = math.Trunc(cpuPercent*1e2+0.5) * 1e-2
			cfm.Memory = mem
			cfm.MemoryPercentage = math.Trunc(memPercent*1e2+0.5) * 1e-2
			cfm.MemoryLimit = memLimit
			cfm.NetworkRx = netRx
			cfm.NetworkTx = netTx
			cfm.BlockRead = float64(blkRead)
			cfm.BlockWrite = float64(blkWrite)
			cfm.PidsCurrent = pidsStatsCurrent
			cfm.ReadTime = statsJSON.Read.Add(time.Hour * 8).Format("2006-01-02 15:04:05")
			cfm.PreReadTime = statsJSON.PreRead.Add(time.Hour * 8).Format("2006-01-02 15:04:05")
			cms.put(cfm)
			u <- nil
			response.Body.Close()
			logger.Println(cfm.ContainerID, cfm.Name, cfm.CPUPercentage, cfm.Memory, cfm.MemoryLimit, cfm.MemoryPercentage, cfm.NetworkRx, cfm.NetworkTx, cfm.BlockRead, cfm.BlockWrite)
			time.Sleep(15 * time.Second)
		}
	}()

	for {
		select {
		case <-time.After(25 * time.Second):
			// zero out the values if we have not received an update within
			// the specified duration.
			//s.SetErrorAndReset(errors.New("timeout waiting for stats"))
			// if this is the first stat you get, release WaitGroup
			if !getFirst {
				getFirst = true
				waitFirst.Done()
			}
		case err := <-u:
			//s.SetError(err)
			if err == io.EOF {
				break
			}
			if err == errNoSuchC {
				logger.Println(cms.name, " is not running, return")
				return
			}

			if err != nil {
				continue
			}
			// if this is the first stat you get, release WaitGroup
			if !getFirst {
				getFirst = true
				waitFirst.Done()
			}
		}

	}
}

func addToAllHostStack(stack *hostContainerMStack) {
	mu.Lock()
	defer mu.Unlock()

	allHostStack = append(allHostStack, stack)
}

func GetContainerMetrics(host, id string) ([]*ContainerFMetrics, error) {
	mu.RLock()
	defer mu.RUnlock()

	for _, hoststack := range allHostStack {
		if hoststack.hostName == host {
			if hoststack.length() == 0 {
				return nil, errors.New("not one container is running")
			}
			for _, containerStack := range hoststack.cms {
				if containerStack.id == id || containerStack.name == id {
					return containerStack.read(5), nil
				}
			}
		}
	}

	return nil, errors.New("given container name or id is unknown, or container is not running")

}

func GetCInfo(host string) []string {
	for _, hoststack := range allHostStack {
		if hoststack.hostName == host {
			if hoststack.length() == 0 {
				return nil
			}
			return hoststack.allNames()
		}
	}

	return nil
}
