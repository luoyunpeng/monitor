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
	logger         *log.Logger
	BufferedCStats = &stats{}
)

type StatsOptions struct {
	all        bool
	noStream   bool
	noTrunc    bool
	format     string
	containers []string
}

func init() {
	file, err := os.OpenFile("container.monitor", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("Failed to open error log file:", err)
	}

	logger = log.New(file, "[MONITOR STATS]: **** ", log.Ldate|log.Ltime|log.Lshortfile)
}

func KeepStats(dockerCli *client.Client) {
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
	getContainerList := func() {
		options := types.ContainerListOptions{
			All: false,
		}
		cs, err := dockerCli.ContainerList(ctx, options)
		if err != nil {
			closeChan <- err
		}
		for _, container := range cs {
			s := NewContainerStats(container.ID[:12])
			if BufferedCStats.add(s) {
				waitFirst.Add(1)
				go collect(ctx, s, dockerCli, waitFirst)
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
		s := NewContainerStats(e.ID[:12])
		if BufferedCStats.add(s) {
			waitFirst.Add(1)
			go collect(ctx, s, dockerCli, waitFirst)
		}
	})

	eh.Handle("die", func(e events.Message) {
		BufferedCStats.remove(e.ID[:12])
	})

	eventChan := make(chan events.Message)
	go eh.Watch(eventChan)
	go monitorContainerEvents(started, eventChan)
	defer close(eventChan)
	// wait event listener go routine started
	<-started

	// Start a short-lived goroutine to retrieve the initial list of
	// containers.
	getContainerList()
	waitFirst.Wait()

	logger.Println("log all running container stats every 4 seconds")
	//record cStats to log files
	for range time.Tick(time.Second * 15) {
		ccstats := []HumanizeStats{}
		BufferedCStats.mu.Lock()
		for _, c := range BufferedCStats.cs {
			ccstats = append(ccstats, *c.GetStatistics())
		}
		BufferedCStats.mu.Unlock()
		logger.Println(ccstats)

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
			// just skip, wait for next log output
		}
	}
}

func collect(ctx context.Context, s *CStats, cli *client.Client, waitFirst *sync.WaitGroup) {
	var (
		getFirst   bool
		u          = make(chan error, 1)
		errNoSuchC = errors.New("no such container")
	)

	defer func() {
		// if error happens and we get nothing of stats, release wait group whatever
		if !getFirst {
			getFirst = true
			waitFirst.Done()
		}
	}()

	go func() {
		for range time.Tick(time.Second * 15) {
			var (
				previousCPU    uint64
				previousSystem uint64

				statsJSON              *types.StatsJSON
				memPercent, cpuPercent float64
				blkRead, blkWrite      uint64 // Only used on Linux
				mem, memLimit          float64
				pidsStatsCurrent       uint64
			)

			response, err := cli.ContainerStats(ctx, s.ContainerID, false)
			if err != nil || s.IsInvalid {
				log.Printf("collecting stats for %v", err)
				if strings.Contains(err.Error(), "No such container") || s.IsInvalid {
					u <- errNoSuchC
				}
				return
			}

			respByte, err := ioutil.ReadAll(response.Body)
			if err != nil {
				log.Printf("collecting stats for %v", err)
				return
			}

			errUnmarshal := json.Unmarshal(respByte, &statsJSON)
			if errUnmarshal != nil {
				log.Printf("Unmarshal collecting stats for %v", errUnmarshal)
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

			s.Name = statsJSON.Name[1:]
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
			u <- nil
			response.Body.Close()
		}
	}()

	for {
		select {
		case <-time.After(25 * time.Second):
			// zero out the values if we have not received an update within
			// the specified duration.
			s.SetErrorAndReset(errors.New("timeout waiting for stats"))
			// if this is the first stat you get, release WaitGroup
			if !getFirst {
				getFirst = true
				waitFirst.Done()
			}
		case err := <-u:
			s.SetError(err)
			if err == io.EOF || err == errNoSuchC {
				break
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

func Getstatics(id string) (*HumanizeStats, error) {
	if len(BufferedCStats.cs) == 0 {
		return nil, errors.New("no container stats")
	}

	var (
		index   int
		isKnown bool
	)
	if index, isKnown = BufferedCStats.isKnownContainer(id); !isKnown {
		return nil, errors.New("given container name or id is unknown, or container is not running")
	}

	return BufferedCStats.cs[index].GetStatistics(), nil
}
