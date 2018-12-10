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
	// Each container can store at most 15 stats record in individual container stack
	// Each Host has many container stack, we use hostStack to store the container stacks
	// AllHostList stores every host's hostStack, the key is host ip address
	AllHostList sync.Map
	// Each host use dockerCli to get stats from docker daemon
	// the dockerCli is stored by DockerCliList
	DockerCliList sync.Map
)

const (
	defaultReadLength      = 15
	defaultCollectDuration = 60 * time.Second
	defaultCollectTimeOut  = defaultCollectDuration + 10*time.Second
	defaultMaxTimeoutTimes = 5
)

func initLog(ip string) *log.Logger {
	file, err := os.OpenFile(ip+".cmonitor", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Failed to open error log file: %v", err)
		return nil
	}

	//return log.New(bufio.NewWriterSize(file, 128*(len(common.HostIPs)+1)), "", log.Ldate|log.Ltime)
	return log.New(file, "", log.Ldate|log.Ltime)
}

// KeepStats keeps monitor all container of the given host
func KeepStats(dockerCli *client.Client, ip string) {
	ctx := context.Background()
	logger := initLog(ip)
	if logger == nil {
		return
	}

	hcmsStack := NewHostContainerMetricStack(ip, logger)
	AllHostList.Store(ip, hcmsStack)

	// monitorContainerEvents watches for container creation and removal (only
	// used when calling `docker stats` without arguments).
	monitorContainerEvents := func(started chan<- struct{}, c chan events.Message) {
		defer func() {
			close(c)
			if dockerCli != nil {
				logger.Println("close docker-cli and remove it from DockerCliList and host list")
				AllHostList.Delete(ip)
				DockerCliList.Delete(ip)
				dockerCli.Close()
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
				logger.Printf("host: err happen when listen docker event: %v", err)
				hcmsStack.StopCollect()
				return
			case <-hcmsStack.Done:
				//logger.Printf("connect to docker daemon error or stop collect by call method, stop container event listener")
				return
			}
		}
	}

	// waitFirst is a WaitGroup to wait first stat data's reach for each container
	waitFirst := &sync.WaitGroup{}

	logger.Println("ID  NAME  CPU %  MEM  USAGE / LIMIT  MEM %  NET I/O  BLOCK I/O READ-TIME")
	// getContainerList simulates creation event for all previously existing
	// containers (only used when calling `docker stats` without arguments).
	getContainerList := func() {
		options := types.ContainerListOptions{
			All: false,
		}
		cs, err := dockerCli.ContainerList(ctx, options)
		if err != nil {
			logger.Printf("err happen when get all running container: %v", err)
			return
		}
		for _, container := range cs {
			_, isKnown := hcmsStack.isKnownContainer(container.ID[:12])
			if isKnown {
				continue
			}
			cms := NewContainerMStack("", container.ID[:12])
			if hcmsStack.Add(cms) {
				waitFirst.Add(1)
				go collect(ctx, cms, dockerCli, waitFirst, hcmsStack)
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
		if hcmsStack.Add(cms) {
			waitFirst.Add(1)
			go collect(ctx, cms, dockerCli, waitFirst, hcmsStack)
		}
	})

	eh.Handle("die", func(e events.Message) {
		logger.Printf("event handler: received die event: %v", e)
		hcmsStack.Remove(e.ID[:12])
		status, err := common.QueryContainerStatus(e.ID)
		if err != nil {
			logger.Printf("query container-%s status error %v", e.ID[:12], err)
			return
		}
		if status == 0 {
			logger.Printf("container-%s status has been 0, no need to change", e.ID[:12])
		} else if status == -100 {
			logger.Printf("no found %s record in table", e.ID[:12])
		} else {
			err := common.ChangeContainerStatus(e.ID, "0")
			if err != nil {
				logger.Printf("change container-%s status error %v", e.ID[:12], err)
				return
			}
			logger.Printf("change container-%s status to 0", e.ID[:12])
		}
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
	logger.Println("container first collecting Done")
}

func collect(ctx context.Context, cms *SingalContainerMetricStack, cli *client.Client, waitFirst *sync.WaitGroup, hcmsStack *HostContainerMetricStack) {
	var (
		isFirstCollect                = true
		lastNetworkTX, lastNetworkRX  float64
		lastBlockRead, lastBlockWrite float64
		cfm                           ParsedConatinerMetrics
		u                             = make(chan error, 1)
		errNoSuchC                    = errors.New("no such container")
		dockerDaemonErr               error
	)

	defer func() {
		// if error happens and we get nothing of stats, release wait group whatever
		if isFirstCollect {
			isFirstCollect = false
			waitFirst.Done()
		}
	}()

	go func() {
		var (
			previousCPU       uint64
			previousSystem    uint64
			statsJSON         types.StatsJSON
			blkRead, blkWrite uint64
			timeFormat        [16]byte
			timeFormatSlice   = timeFormat[:0]
		)

		for {
			select {
			case <-hcmsStack.Done:
				//logger.Printf("collector for  %s  from docker daemon canceled, return", cms.ContainerName)
				return
			default:
				if cms.isInvalid {
					//container stop or rm event happened or others(event that lead to stop the container), return collect goroutine
					u <- errNoSuchC
					return
				}

				response, err := cli.ContainerStats(ctx, cms.ID, false)
				if err != nil {
					dockerDaemonErr = err
					u <- dockerDaemonErr
					return
				}

				respByte, err := ioutil.ReadAll(response.Body)
				if err != nil {
					hcmsStack.logger.Printf("ioutil read from response body for %s err occured: %v", cms.ContainerName, err)
					u <- err
					if err == io.EOF {
						break
					}
					time.Sleep(100 * time.Millisecond)
					continue
				}

				errUnmarshal := json.Unmarshal(respByte, &statsJSON)
				if errUnmarshal != nil {
					hcmsStack.logger.Printf("Unmarshal collecting stats for %s err occured: %v", cms.ContainerName, errUnmarshal)
					u <- errUnmarshal
					if errUnmarshal == io.EOF {
						break
					}
					time.Sleep(100 * time.Millisecond)
					continue
				}

				previousCPU = statsJSON.PreCPUStats.CPUUsage.TotalUsage
				previousSystem = statsJSON.PreCPUStats.SystemUsage
				cfm.CPUPercentage = CalculateCPUPercentUnix(previousCPU, previousSystem, statsJSON)
				blkRead, blkWrite = CalculateBlockIO(statsJSON.BlkioStats)
				// default mem related metric unit is MB
				cfm.Memory = CalculateMemUsageUnixNoCache(statsJSON.MemoryStats)
				cfm.MemoryLimit = Round(float64(statsJSON.MemoryStats.Limit)/(1024*1024), 3)
				cfm.MemoryPercentage = CalculateMemPercentUnixNoCache(cfm.MemoryLimit, cfm.Memory)
				cfm.PidsCurrent = statsJSON.PidsStats.Current
				netRx, netTx := CalculateNetwork(statsJSON.Networks)

				if cms.ContainerName == "" {
					cms.ContainerName = statsJSON.Name[1:]
				}
				if isFirstCollect {
					//network io
					lastNetworkRX, cfm.NetworkRx = netRx, 0
					lastNetworkTX, cfm.NetworkTx = netTx, 0

					//block io
					lastBlockRead, cfm.BlockRead = Round(float64(blkRead/(1024*1024)), 3), 0
					lastBlockWrite, cfm.BlockWrite = Round(float64(blkWrite/(1024*1024)), 3), 0
				} else {
					//network io
					lastNetworkRX, cfm.NetworkRx = netRx, Round(netRx-lastNetworkRX, 3)
					lastNetworkTX, cfm.NetworkTx = netTx, Round(netTx-lastNetworkTX, 3)

					//block io
					tmpRead := Round(float64(blkRead/(1024*1024)), 3)
					tmpWrite := Round(float64(blkWrite/(1024*1024)), 3)
					lastBlockRead, cfm.BlockRead = tmpRead, Round(float64(blkRead/(1024*1024))-lastBlockRead, 3)
					lastBlockWrite, cfm.BlockWrite = tmpWrite, Round(float64(blkWrite/(1024*1024))-lastBlockWrite, 3)
				}
				statsJSON.Read.Add(time.Hour*8).AppendFormat(timeFormatSlice, "15:04:05")
				cfm.ReadTime = string(timeFormat[:8])
				cfm.ReadTimeForInfluxDB = statsJSON.Read //.Add(time.Hour * 8) , if necessary add 8 hours
				cms.Put(cfm)
				u <- nil
				response.Body.Close()
				if !cms.isInvalid {
					hcmsStack.logger.Println(cms.ID, cms.ContainerName, cfm.CPUPercentage, cfm.Memory, cfm.MemoryLimit, cfm.MemoryPercentage, cfm.NetworkRx, cfm.NetworkTx, cfm.BlockRead, cfm.BlockWrite, cfm.ReadTime)
					WriteMetricToInfluxDB(hcmsStack.hostName, cms.ContainerName, cfm)
				}
				time.Sleep(defaultCollectDuration)
			}
		}
	}()

	timeoutTimes := 0
	for {
		t := time.NewTimer(defaultCollectTimeOut)
		select {
		case <-t.C:
			// zero out the values if we have not received an update within
			// the specified duration.
			if timeoutTimes == defaultMaxTimeoutTimes {
				_, err := cli.Ping(ctx)
				if err != nil {
					hcmsStack.logger.Printf("time out for collect "+cms.ContainerName+" reach the top times, err of Ping is: %v", err)
				}
				hcmsStack.StopCollect()
				return
			}
			// if this is the first stat you get, release WaitGroup
			if isFirstCollect {
				isFirstCollect = false
				waitFirst.Done()
			}
			timeoutTimes++
			hcmsStack.logger.Println("collect for container-"+cms.ContainerName, " time out for "+strconv.Itoa(timeoutTimes)+" times")
		case err := <-u:
			t.Stop()
			//EOF error maybe mean docker daemon err
			if err == io.EOF {
				break
			}

			if err == errNoSuchC {
				//hcmsStack.logger.Println(cms.ContainerName, " is not running, stop collecting in goroutine")
				return
			} else if err != nil && err == dockerDaemonErr {
				hcmsStack.logger.Printf("collecting stats from daemon for "+cms.ContainerName+" error occured: %v", err)
				//if strings.Contains(err.Error(),"Is the docker daemon running?") {
				//	hcmsStack.cancel()
				//}
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
		case <-hcmsStack.Done:
			t.Stop()
			return
		}
	}
}

// GetContainerMetrics return container stats
func GetContainerMetrics(host, id string) ([]ParsedConatinerMetrics, error) {
	if hoststackTmp, ok := AllHostList.Load(host); ok {
		if hoststack, ok := hoststackTmp.(*HostContainerMetricStack); ok {
			for _, containerStack := range hoststack.cms {
				if containerStack.ID == id || (len(id) >= 12 && containerStack.ID == id[:12]) || containerStack.ContainerName == id {
					return containerStack.Read(defaultReadLength), nil
				}
			}
			return nil, errors.New("given container name or id is unknown, or container is not running")
		}
	}

	return nil, errors.New("given host " + host + " is not loaded")
}

// GetHostContainerInfo return Host's container info
func GetHostContainerInfo(host string) []string {
	if hoststackTmp, ok := AllHostList.Load(host); ok {
		if hoststack, ok := hoststackTmp.(*HostContainerMetricStack); ok {
			if hoststack.hostName == host {
				return hoststack.AllNames()
			}
		}
	}

	return nil
}

// WriteMetricToInfluxDB write docker container metric to influxDB
func WriteMetricToInfluxDB(host, containerName string, containerMetrics ParsedConatinerMetrics) {
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

	createMetricAndWrite(measurement, tags, fields, containerMetrics.ReadTimeForInfluxDB)
}

type singalHostInfo struct {
	Name                  string
	Images                int
	TotalContainer        int
	TotalRunningContainer int
	TotalStoppedContainer int
	NCPU                  int
	TotalMem              int64
	KernelVersion         string
	OS                    string
}

// WriteDockerHostInfoToInfluxDB write Docker host info to influxDB
func WriteDockerHostInfoToInfluxDB(host string, info singalHostInfo, logger *log.Logger) {
	measurement := "dockerHostInfo"
	fields := make(map[string]interface{})
	tags := map[string]string{
		"host": host,
	}

	fields["hostName"] = info.Name
	fields["imagesLen"] = info.Images
	fields["containerTotal"] = info.TotalContainer
	fields["containerRunning"] = info.TotalRunningContainer
	fields["containersStopped"] = info.TotalStoppedContainer
	fields["ncpu"] = info.NCPU
	fields["totalMem"] = Round(float64(info.TotalMem)/(1024*1024*1024), 2)
	fields["kernelVersion"] = info.KernelVersion
	fields["os"] = info.OS
	if hoststackTmp, ok := AllHostList.Load(host); ok {
		if hoststack, ok := hoststackTmp.(*HostContainerMetricStack); ok {
			if hoststack.hostName == host {
				fields["ContainerMemUsedPercentage"] = Round(hoststack.GetAllLastMemory()*100/float64(info.TotalMem/(1024*1024)), 2)
			}
		}
	}

	createMetricAndWrite(measurement, tags, fields, time.Now())
	logger.Println("write "+host+"  info: ",
		fields["hostName"], fields["imagesLen"], fields["containerTotal"], fields["containerRunning"],
		fields["containersStopped"], fields["ncpu"], fields["totalMem"], fields["kernelVersion"],
		fields["os"], fields["ContainerMemUsedPercentage"])
}

// Calculate all docker host info and write to influxDB
func WriteAllHostInfo() {
	var (
		runningDockerHost, totalContainer, totalRunningContainer int
		measurement                                              = "allHost"
		info                                                     types.Info
		hostInfo                                                 singalHostInfo

		infoErr error
	)
	fields := make(map[string]interface{})
	tags := map[string]string{
		"ALL": "all",
	}
	logger := initLog("all-host")

	ticker := time.Tick(defaultCollectDuration * 5)
	go common.Write()

	for range ticker {
		runningDockerHost = 0
		totalContainer = 0
		totalRunningContainer = 0
		DockerCliList.Range(func(key, cliTmp interface{}) bool {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()
			if cli, ok := cliTmp.(*client.Client); ok {
				ip, _ := key.(string)
				info, infoErr = cli.Info(ctx)
				if infoErr != nil {
					logger.Printf(ip+" get docker info error occured: %v", infoErr)
					return true
				}
				runningDockerHost++
				hostInfo = singalHostInfo{
					Name:                  info.Name,
					Images:                info.Images,
					TotalContainer:        info.Containers,
					TotalRunningContainer: info.ContainersRunning,
					NCPU:                  info.NCPU,
					TotalMem:              info.MemTotal,
					KernelVersion:         info.KernelVersion,
					OS:                    info.OperatingSystem + info.Architecture,
				}
				hostInfo.TotalStoppedContainer = hostInfo.TotalContainer - hostInfo.TotalRunningContainer
				totalContainer += info.Containers
				totalRunningContainer += info.ContainersRunning
				WriteDockerHostInfoToInfluxDB(ip, hostInfo, logger)
			}

			return true
		})
		if runningDockerHost == 0 {
			logger.Println("no more docker daemon is running, return store all host info to influxDB")
			return
		}
		fields["hostNum"] = len(common.HostIPs)
		fields["dockerdRunning"] = runningDockerHost
		fields["dockerdDead"] = len(common.HostIPs) - runningDockerHost
		fields["totalContainer"] = totalContainer
		fields["totalRunning"] = totalRunningContainer
		fields["totalStopped"] = totalContainer - totalRunningContainer

		createMetricAndWrite(measurement, tags, fields, time.Now())
		logger.Printf("write all host info: %d, %d, %d, %d, %d, %d",
			fields["hostNum"], runningDockerHost, fields["dockerdDead"], totalContainer,
			totalRunningContainer, fields["totalStopped"])
	}
}

func createMetricAndWrite(measurement string, tags map[string]string, fields map[string]interface{}, readTime time.Time) {
	m := common.Metric{}
	m.Measurement = measurement
	m.Tags = tags
	m.Fields = fields
	m.ReadTime = readTime
	common.MetricChan <- m
}
