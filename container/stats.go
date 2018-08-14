package container

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/luoyunpeng/monitor/common"
)

var (
	AllHostList   sync.Map
	DockerCliList sync.Map
)

const (
	defaultReadLength      = 15
	defaultCollectDuration = 15 * time.Second
	defaultCollectTimeOut  = defaultCollectDuration + 10*time.Second
	defaultMaxTimeoutTimes = 5
)

func initLog(ip string) *log.Logger {
	file, err := os.OpenFile(ip+".cmonitor", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("Failed to open error log file:", err)
	}

	return log.New(file, "", log.Ldate|log.Ltime)
}

//each host will run this method only once
func KeepStats(dockerCli *client.Client, ip string) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := initLog(ip)
	// monitorContainerEvents watches for container creation and removal (only
	// used when calling `docker stats` without arguments).
	monitorContainerEvents := func(started chan<- struct{}, c chan events.Message) {
		defer func() {
			close(c)
			if dockerCli != nil {
				logger.Println("close docker-cli-" + ip + ", and remove it from DockerCliList and host list ")
				dockerCli.Close()
				DockerCliList.Delete(ip)
				AllHostList.Delete(ip)
			}
		}()

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
				logger.Printf("host:"+ip+" err happen when listen docker event: %v", err)
				cancel()
				return
			case <-ctx.Done():
				logger.Printf("connect to docker daemon: " + ip + " time out, stop container event listener")
				return
			}
		}
	}

	// waitFirst is a WaitGroup to wait first stat data's reach for each container
	waitFirst := &sync.WaitGroup{}

	hcmsStack := NewHostContainerMetricStack(ip)
	AllHostList.Store(ip, hcmsStack)
	logger.Println("ID  NAME  CPU %  MEM  USAGE / LIMIT  MEM %  NET I/O  BLOCK I/O READ-TIME")
	// getContainerList simulates creation event for all previously existing
	// containers (only used when calling `docker stats` without arguments).
	getContainerList := func() {
		options := types.ContainerListOptions{
			All: false,
		}
		cs, err := dockerCli.ContainerList(ctx, options)
		if err != nil {
			logger.Printf("host:"+ip+" err happen when get all running container: %v", err)
			return
		}
		for _, container := range cs {
			cms := NewContainerMStack("", container.ID[:12])
			if hcmsStack.add(cms) {
				waitFirst.Add(1)
				go collect(ctx, cms, dockerCli, waitFirst, logger, cancel, ip)
			}
		}
	}

	// default list all running containers, start a long running goroutine which
	// monitors container events. We make sure we're subscribed before
	// retrieving the list of running containers to avoid a race where we
	// would "miss" a creation.
	started := make(chan struct{})
	eh := InitEventHandler()
	eh.Handle("start", func(e events.Message) {
		logger.Printf("event handler: received start event: %v", e)
		cms := NewContainerMStack("", e.ID[:12])
		if hcmsStack.add(cms) {
			waitFirst.Add(1)
			go collect(ctx, cms, dockerCli, waitFirst, logger, cancel, ip)
		}
	})

	eh.Handle("die", func(e events.Message) {
		logger.Printf("event handler: received die event: %v", e)
		hcmsStack.remove(e.ID[:12])
	})

	eventChan := make(chan events.Message)
	go eh.Watch(eventChan)
	go monitorContainerEvents(started, eventChan)
	// wait event listener go routine started
	<-started

	// Start a short-lived goroutine to retrieve the initial list of
	// containers.
	getContainerList()
	waitFirst.Wait()
	logger.Println("host-" + ip + " collect for all running container successfull")
	go WriteDockerHostInfoToInfluxDB(ctx, dockerCli, ip, logger)
}

func collect(ctx context.Context, cms *containerMetricStack, cli *client.Client, waitFirst *sync.WaitGroup, logger *log.Logger, cancel context.CancelFunc, host string) {
	var (
		isFirstCollect                = true
		cfm                           *ParsedConatinerMetrics
		lastNetworkTX, lastNetworkRX  float64
		lastBlockRead, lastBlockWrite float64

		u               = make(chan error, 1)
		errNoSuchC      = errors.New("no such container")
		dockerDaemonErr error
	)

	defer func() {
		// if error happens and we get nothing of stats, release wait group whatever
		if isFirstCollect {
			isFirstCollect = false
			waitFirst.Done()
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.Println("collector for  " + cms.ContainerName + " from docker daemon canceled, return")
				return
			default:
				var (
					previousCPU    uint64
					previousSystem uint64

					statsJSON              *types.StatsJSON
					memPercent, cpuPercent float64
					blkRead, blkWrite      uint64
					mem, memLimit          float64
					pidsStatsCurrent       uint64
				)

				response, err := cli.ContainerStats(ctx, cms.ID, false)
				// bool value initial value is false
				if cms.isInvalid {
					if response.Body != nil {
						response.Body.Close()
					}
					//container stop or rm event happened or others(event that lead to stop the container), return collecting goroutine
					u <- errNoSuchC
					return
				}
				if err != nil {
					dockerDaemonErr = err
					u <- dockerDaemonErr
					return
				}

				respByte, err := ioutil.ReadAll(response.Body)
				if err != nil {
					logger.Printf("ioutil read from response body  for "+cms.ContainerName+" err occured: %v", err)
					u <- err
					if err == io.EOF {
						break
					}
					time.Sleep(100 * time.Millisecond)
					continue
				}

				errUnmarshal := json.Unmarshal(respByte, &statsJSON)
				if errUnmarshal != nil {
					logger.Printf("Unmarshal collecting stats for "+cms.ContainerName+" err occured: %v", errUnmarshal)
					u <- errUnmarshal
					if errUnmarshal == io.EOF {
						break
					}
					time.Sleep(100 * time.Millisecond)
					continue
				}

				previousCPU = statsJSON.PreCPUStats.CPUUsage.TotalUsage
				previousSystem = statsJSON.PreCPUStats.SystemUsage
				cpuPercent = CalculateCPUPercentUnix(previousCPU, previousSystem, statsJSON)
				blkRead, blkWrite = CalculateBlockIO(statsJSON.BlkioStats)
				// default mem related metric unit is MB
				mem = CalculateMemUsageUnixNoCache(statsJSON.MemoryStats)
				memLimit = Round(float64(statsJSON.MemoryStats.Limit)/(1024*1024), 3)
				memPercent = CalculateMemPercentUnixNoCache(memLimit, mem)
				pidsStatsCurrent = statsJSON.PidsStats.Current
				netRx, netTx := CalculateNetwork(statsJSON.Networks)

				cfm = NewParsedConatinerMetrics()
				if cms.ContainerName == "" {
					cms.ContainerName = statsJSON.Name[1:]
				}
				cfm.CPUPercentage = cpuPercent
				cfm.Memory = mem
				cfm.MemoryPercentage = memPercent
				cfm.MemoryLimit = memLimit
				if isFirstCollect {
					lastNetworkRX, cfm.NetworkRx = netRx, 0
					lastNetworkTX, cfm.NetworkTx = netTx, 0
				} else {
					lastNetworkRX, cfm.NetworkRx = netRx, Round(netRx-lastNetworkRX, 3)
					lastNetworkTX, cfm.NetworkTx = netTx, Round(netTx-lastNetworkTX, 3)
				}

				if isFirstCollect {
					lastBlockRead, cfm.BlockRead = Round(float64(blkRead)/(1024*1024), 3), 0
					lastBlockWrite, cfm.BlockWrite = Round(float64(blkWrite)/(1024*1024), 3), 0
				} else {
					tmpRead := Round(float64(blkRead)/(1024*1024), 3)
					tmpWrite := Round(float64(blkWrite)/(1024*1024), 3)
					lastBlockRead, cfm.BlockRead = tmpRead, Round(float64(blkRead)/(1024*1024)-lastBlockRead, 3)
					lastBlockWrite, cfm.BlockWrite = tmpWrite, Round(float64(blkWrite)/(1024*1024)-lastBlockWrite, 3)
				}
				cfm.PidsCurrent = pidsStatsCurrent
				cfm.ReadTime = statsJSON.Read.Add(time.Hour * 8).Format("2006-01-02 15:04:05")
				cfm.ReadTimeForInfluxDB = statsJSON.Read.Add(time.Hour * 8)
				cms.put(cfm)
				u <- nil
				response.Body.Close()
				logger.Println(cms.ID, cms.ContainerName, cfm.CPUPercentage, cfm.Memory, cfm.MemoryLimit, cfm.MemoryPercentage, cfm.NetworkRx, cfm.NetworkTx, cfm.BlockRead, cfm.BlockWrite, cfm.ReadTime)
				go WriteMetricToInfluxDB(host, cms.ContainerName, cfm)
				time.Sleep(defaultCollectDuration)
			}
		}
	}()

	timeoutTimes := 0
	for {
		select {
		case <-time.After(defaultCollectTimeOut):
			// zero out the values if we have not received an update within
			// the specified duration.
			if timeoutTimes > defaultMaxTimeoutTimes {
				_, err := cli.Ping(ctx)
				if err != nil {
					logger.Printf("time out for collect "+cms.ContainerName+" reach the top times, err of Ping is: %v", err)
				}
				cancel()
				return
			}
			// if this is the first stat you get, release WaitGroup
			if isFirstCollect {
				isFirstCollect = false
				waitFirst.Done()
			}
			timeoutTimes++
			logger.Println("collect for container-"+cms.ContainerName, " time out for "+strconv.Itoa(timeoutTimes)+" times")
		case err := <-u:
			//EOF error maybe mean docker daemon err
			if err == io.EOF {
				break
			}

			if err == errNoSuchC {
				logger.Println(cms.ContainerName, " is not running, stop collecting in goroutine")
				return
			} else if err != nil && err == dockerDaemonErr {
				logger.Printf("collecting stats from daemon for "+cms.ContainerName+" error occured: %v", err)
				return
			}
			if err != nil {
				continue
			}
			//if err is nil mean collect metrics successfully
			// if this is the first stat you get, release WaitGroup
			if isFirstCollect {
				isFirstCollect = false
				waitFirst.Done()
			}
		case <-ctx.Done():
			return
		}
	}
}

func GetContainerMetrics(host, id string) ([]*ParsedConatinerMetrics, error) {
	if hoststackTmp, ok := AllHostList.Load(host); ok {
		if hoststack, ok := hoststackTmp.(*HostContainerMetricStack); ok {
			for _, containerStack := range hoststack.cms {
				if containerStack.ID == id || containerStack.ContainerName == id {
					return containerStack.read(defaultReadLength), nil
				}
			}
			return nil, errors.New("given container name or id is unknown, or container is not running")
		}
	}
	return nil, errors.New("given host " + host + " is not loaded")
}

func GetHostContainerInfo(host string) []string {
	if hoststackTmp, ok := AllHostList.Load(host); ok {
		if hoststack, ok := hoststackTmp.(*HostContainerMetricStack); ok {
			if hoststack.hostName == host {
				return hoststack.allNames()
			}
		}
	}

	return nil
}

func WriteMetricToInfluxDB(host, containerName string, containerMetrics *ParsedConatinerMetrics) {
	fields := make(map[string]interface{})
	measurement := "container"
	fieldKeys := []string{"cpu", "mem", "memLimit", "networkTX", "networkRX", "blockRead", "blockWrite"}
	tags := map[string]string{
		"host": host,
		"name": containerName,
	}

	for _, fKey := range fieldKeys {
		switch fKey {
		case "cpu":
			fields[fKey] = containerMetrics.CPUPercentage
		case "mem":
			fields[fKey] = containerMetrics.Memory
		case "memLimit":
			fields[fKey] = containerMetrics.MemoryLimit
		case "networkTX":
			fields[fKey] = containerMetrics.NetworkTx
		case "networkRX":
			fields[fKey] = containerMetrics.NetworkRx
		case "blockRead":
			fields[fKey] = containerMetrics.BlockRead
		case "blockWrite":
			fields[fKey] = containerMetrics.BlockWrite
		}
	}

	go common.Write(measurement, tags, fields, containerMetrics.ReadTimeForInfluxDB)
}

func WriteDockerHostInfoToInfluxDB(ctx context.Context, cli *client.Client, host string, logger *log.Logger) {
	measurement := "dockerHostInfo"
	fields := make(map[string]interface{})
	tags := map[string]string{
		"host": host,
	}
	//TODO, created, restarting, running, removing, paused, exited and dead ,for show docker container status on grafana
	for {
		select {
		case <-ctx.Done():
			logger.Println("cancel for writing docker host- " + host + " info to influxdb")
			return
		default:
			info, err := cli.Info(ctx)
			if err != nil {
				logger.Printf("get docker host info error occured: %v", err)
				return
			}
			fields["hostName"] = info.Name
			fields["imagesLen"] = info.Images
			fields["containerTotal"] = info.Containers
			fields["containerRunning"] = info.ContainersRunning
			fields["containersStopped"] = info.ContainersStopped
			fields["ncpu"] = info.NCPU
			fields["totalMem"] = Round(float64(info.MemTotal)/(1024*1024*1024), 2)
			fields["kernelVersion"] = info.KernelVersion
			fields["os"] = info.OperatingSystem + info.Architecture
			if hoststackTmp, ok := AllHostList.Load(host); ok {
				if hoststack, ok := hoststackTmp.(*HostContainerMetricStack); ok {
					if hoststack.hostName == host {
						fields["ContainerMemUsedPercentage"] = Round(hoststack.getAllLastMemory()*100/float64(info.MemTotal/(1024*1024)), 2)
					}
				}
			}
			go common.Write(measurement, tags, fields, time.Now())
			time.Sleep(defaultCollectDuration)
		}
	}
}

func WriteAllHostInfo() {
	time.Sleep(defaultCollectDuration * 2)
	measurement := "allHost"
	fields := make(map[string]interface{})
	tags := map[string]string{
		"ALL": "all",
	}

	for {
		stopped := 0
		running := 0
		DockerCliList.Range(func(key, cliTmp interface{}) bool {
			if cli, ok := cliTmp.(*client.Client); ok {
				_, err := cli.Ping(context.Background())
				if err != nil {
					k, _ := key.(string)
					println(" ping host-"+k+" err:", err, " when store all host info to influxdb")
					stopped++
				} else {
					running++
				}
			}
			return true
		})
		fields["hostNum"] = len(common.HostIPs)
		fields["dockerdRunning"] = running
		fields["dockerdDead"] = len(common.HostIPs) - running
		go common.Write(measurement, tags, fields, time.Now())
		if running == 0 {
			println("no more docker daemono is running, return store all host info to influxdb")
			return
		}
		time.Sleep(defaultCollectDuration)
	}
}
