package monitor

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	jsoniter "github.com/json-iterator/go"
	"github.com/luoyunpeng/monitor/internal/config"
	"github.com/luoyunpeng/monitor/internal/models"
	"github.com/luoyunpeng/monitor/internal/util"
)

var (
	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

// KeepStats keeps monitor all container of the given host
func Monitor(dockerCli *client.Client, ip string) {
	ctx := context.Background()
	logger := util.InitLog(ip)
	if logger == nil {
		return
	}

	dh := models.NewDockerHost(ip, logger)
	dh.Cli = dockerCli
	models.DockerHostCache.Store(ip, dh)

	// monitorContainerEvents watches for container creation and removal (only
	// used when calling `docker stats` without arguments).
	monitorContainerEvents := func(started chan<- struct{}, c chan events.Message) {
		defer func() {
			close(c)
			if dockerCli != nil {
				logger.Println("close docker-cli and remove it from DockerCliList and host list")
				models.DockerHostCache.Delete(ip)
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
				dh.StopCollect()
				return
			case <-dh.Done:
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
			cms := models.NewCMetric("", container.ID[:12])
			if dh.Add(cms) {
				waitFirst.Add(1)
				go collect(cms, waitFirst, dh)
			}
		}
	}

	// default list all running containers, start a long running goroutine which
	// monitors container events. We make sure we're subscribed before
	// retrieving the list of running containers to avoid a race where we
	// would "miss" a creation.
	started := make(chan struct{})
	eh := util.InitEventHandler()
	eh.Handle("start", func(e events.Message) {
		cms := models.NewCMetric(e.Actor.Attributes["name"], e.ID[:12])
		logger.Printf("event handler: received %s-start event: %v", cms.ContainerName, e)
		if dh.Add(cms) {
			waitFirst.Add(1)
			go collect(cms, waitFirst, dh)
		}
	})

	eh.Handle("die", func(e events.Message) {
		logger.Printf("event handler: received die event: %v", e)
		dh.Remove(e.ID[:12])
		status, err := QueryContainerStatus(e.ID)
		if err != nil {
			logger.Printf("query container-%s status error %v", e.ID[:12], err)
			return
		}
		if status == 0 {
			logger.Printf("container-%s status has been 0, no need to change", e.ID[:12])
		} else if status == -100 {
			logger.Printf("no found %s record in table", e.ID[:12])
		} else {
			if dh.IsValid() {
				err := ChangeContainerStatus(e.ID, "0")
				if err != nil {
					logger.Printf("change container-%s status error %v", e.ID[:12], err)
					return
				}
				logger.Printf("change container-%s status to 0", e.ID[:12])
				return
			}
			logger.Printf("host-%s id done, not write container-%s stats to mysql", dh.GetIP(), e.ID[:12])
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

func collect(cm *models.ContainerStats, waitFirst *sync.WaitGroup, dh *models.DockerHost) {
	var (
		isFirstCollect                = true
		lastNetworkTX, lastNetworkRX  float64
		lastBlockRead, lastBlockWrite float64
		cfm                           models.ParsedConatinerMetric
		u                             = make(chan error, 1)
		errNoSuchC                    = errors.New("no such container")
		dockerDaemonErr               error
		ctx                           = context.Background()
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

			bufferReader = bufio.NewReaderSize(nil, 512)
			decoder      = json.NewDecoder(bufferReader)
		)

		for {
			select {
			case <-dh.Done:
				//logger.Printf("collector for  %s  from docker daemon canceled, return", cms.ContainerName)
				return
			default:
				if cm.IsInValid() {
					//container stop or rm event happened or others(event that lead to stop the container), return collect goroutine
					u <- errNoSuchC
					return
				}

				response, err := dh.Cli.ContainerStats(ctx, cm.ID, false)
				if err != nil && strings.Contains(err.Error(), "No such container") {
					log.Printf("container-%s die event happend after calling stats", cm.ID)
					u <- errNoSuchC
					return
				} else if err != nil {
					dockerDaemonErr = err
					u <- dockerDaemonErr
					return
				}
				bufferReader.Reset(response.Body)

				errD := decoder.Decode(&statsJSON)
				if errD != nil {
					dh.Logger.Printf("Decode collecting stats for %s err occured: %v", cm.ContainerName, errD)
				}

				previousCPU = statsJSON.PreCPUStats.CPUUsage.TotalUsage
				previousSystem = statsJSON.PreCPUStats.SystemUsage
				cfm.CPUPercentage = util.CalculateCPUPercentUnix(previousCPU, previousSystem, statsJSON)
				blkRead, blkWrite = util.CalculateBlockIO(statsJSON.BlkioStats)
				// default mem related metric unit is MB
				cfm.Memory = util.CalculateMemUsageUnixNoCache(statsJSON.MemoryStats)
				cfm.MemoryLimit = util.Round(float64(statsJSON.MemoryStats.Limit)/(1024*1024), 3)
				cfm.MemoryPercentage = util.CalculateMemPercentUnixNoCache(cfm.MemoryLimit, cfm.Memory)
				cfm.PidsCurrent = statsJSON.PidsStats.Current
				netRx, netTx := util.CalculateNetwork(statsJSON.Networks)

				if cm.ContainerName == "" && len(statsJSON.Name) >= 2 {
					cm.ContainerName = statsJSON.Name[1:]
				} else if len(statsJSON.Name) <= 2 {
					dh.Logger.Printf("container-%s get short or zero len name-%s", cm.ID, statsJSON.Name)
				}
				if isFirstCollect {
					//network io
					lastNetworkRX, cfm.NetworkRx = netRx, 0
					lastNetworkTX, cfm.NetworkTx = netTx, 0

					//block io
					lastBlockRead, cfm.BlockRead = util.Round(float64(blkRead/(1024*1024)), 3), 0
					lastBlockWrite, cfm.BlockWrite = util.Round(float64(blkWrite/(1024*1024)), 3), 0
				} else {
					//network io
					lastNetworkRX, cfm.NetworkRx = netRx, util.Round(netRx-lastNetworkRX, 3)
					lastNetworkTX, cfm.NetworkTx = netTx, util.Round(netTx-lastNetworkTX, 3)

					//block io
					tmpRead := util.Round(float64(blkRead/(1024*1024)), 3)
					tmpWrite := util.Round(float64(blkWrite/(1024*1024)), 3)
					lastBlockRead, cfm.BlockRead = tmpRead, util.Round(float64(blkRead/(1024*1024))-lastBlockRead, 3)
					lastBlockWrite, cfm.BlockWrite = tmpWrite, util.Round(float64(blkWrite/(1024*1024))-lastBlockWrite, 3)
				}
				statsJSON.Read.Add(time.Hour*8).AppendFormat(timeFormatSlice, "15:04:05")
				cfm.ReadTime = string(timeFormat[:8])
				cfm.ReadTimeForInfluxDB = statsJSON.Read //.Add(time.Hour * 8) , if necessary add 8 hours
				cm.Put(cfm)
				u <- nil
				response.Body.Close()
				if !cm.IsInValid() {
					//dh.logger.Println(cm.ID, cm.ContainerName, cfm.CPUPercentage, cfm.Memory, cfm.MemoryLimit, cfm.MemoryPercentage, cfm.NetworkRx, cfm.NetworkTx, cfm.BlockRead, cfm.BlockWrite, cfm.ReadTime)
					WriteMetricToInfluxDB(dh.GetIP(), cm.ContainerName, cfm)
				}
				time.Sleep(config.MonitorInfo.CollectDuration)
			}
		}
	}()

	timeoutTimes := 0
	t := time.NewTimer(config.MonitorInfo.CollectTimeout)
	for {
		select {
		case <-t.C:
			// zero out the values if we have not received an update within
			// the specified duration.
			if timeoutTimes == config.MonitorInfo.MaxTimeoutTimes {
				_, err := dh.Cli.Ping(ctx)
				if err != nil {
					dh.Logger.Printf("time out for collecting "+cm.ContainerName+" reach the top times, err of Ping is: %v", err)
					dh.StopCollect()
					t.Stop()
					return
				}
				timeoutTimes = 0
				t.Reset(config.MonitorInfo.CollectTimeout)
				continue
			}
			// if this is the first stat you get, release WaitGroup
			if isFirstCollect {
				isFirstCollect = false
				waitFirst.Done()
			}
			timeoutTimes++
			dh.Logger.Println("collect for container-"+cm.ContainerName, " time out for "+strconv.Itoa(timeoutTimes)+" times")
			t.Reset(config.MonitorInfo.CollectTimeout)
		case err := <-u:
			//EOF error maybe mean docker daemon err
			if err == io.EOF {
				break
			}

			if err == errNoSuchC {
				//hcmsStack.logger.Println(cms.ContainerName, " is not running, stop collecting in goroutine")
				t.Stop()
				return
			} else if err != nil && err == dockerDaemonErr {
				dh.Logger.Printf("collecting stats from daemon for "+cm.ContainerName+" error occured: %v", err)
				dh.StopCollect()
				t.Stop()
				return
			}
			if err != nil {
				t.Reset(config.MonitorInfo.CollectTimeout)
				continue
			}
			//if err is nil mean collect metrics successfully
			// if this is the first stat you get, release WaitGroup
			if isFirstCollect {
				isFirstCollect = false
				waitFirst.Done()
			}
			t.Reset(config.MonitorInfo.CollectTimeout)
		case <-dh.Done:
			t.Stop()
			return
		}
	}
}

// GetHostContainerInfo return Host's container info
func GetHostContainerInfo(ip string) []string {
	if hoststackTmp, ok := models.DockerHostCache.Load(ip); ok {
		if dh, ok := hoststackTmp.(*models.DockerHost); ok {
			if dh.GetIP() == ip {
				return dh.AllNames()
			}
		}
	}

	return nil
}

// WriteMetricToInfluxDB write docker container metric to influxDB
func WriteMetricToInfluxDB(host, containerName string, containerMetrics models.ParsedConatinerMetric) {
	measurement := "container"
	fieldKeys := []string{"cpu", "mem", "memLimit", "networkTX", "networkRX", "blockRead", "blockWrite"}

	fields := make(map[string]interface{}, 7)
	tags := make(map[string]string, 2)
	tags["host"] = host
	tags["name"] = containerName

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
func WriteDockerHostInfoToInfluxDB(host string, info singalHostInfo) {
	measurement := "dockerHostInfo"
	fields := make(map[string]interface{}, 10)
	tags := map[string]string{
		"host": host,
	}

	fields["hostName"] = info.Name
	fields["imagesLen"] = info.Images
	fields["containerTotal"] = info.TotalContainer
	fields["containerRunning"] = info.TotalRunningContainer
	fields["containersStopped"] = info.TotalStoppedContainer
	fields["ncpu"] = info.NCPU
	fields["totalMem"] = util.Round(float64(info.TotalMem)/(1024*1024*1024), 2)
	fields["kernelVersion"] = info.KernelVersion
	fields["os"] = info.OS
	if hoststackTmp, ok := models.DockerHostCache.Load(host); ok {
		if dh, ok := hoststackTmp.(*models.DockerHost); ok {
			if dh.GetIP() == host {
				fields["ContainerMemUsedPercentage"] = util.Round(dh.GetAllLastMemory()*100/float64(info.TotalMem/(1024*1024)), 2)
			}
		}
	}

	createMetricAndWrite(measurement, tags, fields, time.Now())
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
	fields := make(map[string]interface{}, 6)
	tags := map[string]string{
		"ALL": "all",
	}
	logger := util.InitLog("all-host")

	ticker := time.NewTicker(5 * config.MonitorInfo.CollectDuration)
	defer ticker.Stop()

	go Write()

	for range ticker.C {
		runningDockerHost = 0
		totalContainer = 0
		totalRunningContainer = 0
		models.DockerHostCache.Range(func(key, value interface{}) bool {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()
			if dh, ok := value.(*models.DockerHost); ok && dh.IsValid() {
				info, infoErr = dh.Cli.Info(ctx)
				if infoErr != nil {
					logger.Printf("%s get docker info error occured: %v", dh.GetIP(), infoErr)
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
				WriteDockerHostInfoToInfluxDB(dh.GetIP(), hostInfo)
			}

			return true
		})
		if runningDockerHost == 0 {
			logger.Println("no more docker daemon is running, return store all host info to influxDB")
			return
		}
		fields["hostNum"] = len(config.MonitorInfo.Hosts)
		fields["dockerdRunning"] = runningDockerHost
		fields["dockerdDead"] = len(config.MonitorInfo.Hosts) - runningDockerHost
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
	m := Metric{}
	m.Measurement = measurement
	m.Tags = tags
	m.Fields = fields
	m.ReadTime = readTime
	MetricChan <- m
}