package handler

import (
	"bufio"
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/luoyunpeng/monitor/common"
	"github.com/luoyunpeng/monitor/container"
)

type RepMetric struct {
	StatusCode int    `json:"statusCode"`
	Status     int    `json:"status"`
	Msg        string `json:"msg"`

	Metric interface{} `json:"metric"`
}

// ContainerStats handles GET requests on /container/stats/:id?host=<hostName>
// if id (container id or name) and host is present, response all metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerStats(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
		return
	}

	hstats, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
		return
	}
	ctx.JSONP(http.StatusOK, hstats)
}

// ContainerMem handles GET requests on /container/metric/mem/:id?host=<hostName>
// if id (container id or name) and host is present, response mem metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerMem(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	var cMem []struct {
		Mem      float64
		ReadTime string
	}

	for _, cm := range csm {
		cMem = append(cMem, struct {
			Mem      float64
			ReadTime string
		}{Mem: cm.Memory, ReadTime: cm.ReadTime})
	}
	ctx.JSONP(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cMem})
}

// ContainerMemPercent handles GET requests on /container/metric/mempercent/:id?host=<hostName>
// if id (container id or name) and host is present, response memory usage metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerMemPercent(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	var cMemPercent struct {
		UsedPercentage  float64
		UnUsePercentage float64
		ReadTime        string
	}

	if len(csm) >= 1 {
		cMemPercent.UsedPercentage = csm[len(csm)-1].MemoryPercentage
		cMemPercent.UnUsePercentage = container.Round(100-cMemPercent.UsedPercentage, 3)
		cMemPercent.ReadTime = csm[len(csm)-1].ReadTime
	}

	ctx.JSONP(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cMemPercent})
}

// ContainerMemLimit handles GET requests on /container/metric/memlimit/:id?host=<hostName>
// if id (container id or name) and host is present, response memory limit metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerMemLimit(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	var cMemLimit struct {
		MemoryLimit float64
		ReadTime    string
	}

	if len(csm) >= 1 {
		cMemLimit.MemoryLimit = csm[len(csm)-1].MemoryLimit
		cMemLimit.ReadTime = csm[len(csm)-1].ReadTime
	}

	ctx.JSONP(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cMemLimit})
}

// ContainerCPU handles GET requests on /container/metric/cpu/:id?host=<hostName>
// if id (container id or name) and host is present, response cpu usage metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerCPU(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	var cCPU []struct {
		CPU      float64
		ReadTime string
	}

	for _, cm := range csm {
		cCPU = append(cCPU, struct {
			CPU      float64
			ReadTime string
		}{CPU: cm.CPUPercentage, ReadTime: cm.ReadTime})
	}

	ctx.JSONP(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cCPU})
}

// ContainerNetworkIO handles GET requests on /container/metric/networkio/:id?host=<hostName>
// if id (container id or name) and host is present, response networkIO metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerNetworkIO(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	var cNetworkIO []struct {
		NetworkTX float64
		NetworkRX float64
		ReadTime  string
	}

	for _, cm := range csm {
		cNetworkIO = append(cNetworkIO, struct {
			NetworkTX float64
			NetworkRX float64
			ReadTime  string
		}{NetworkTX: cm.NetworkTx, NetworkRX: cm.NetworkRx, ReadTime: cm.ReadTime})
	}

	ctx.JSONP(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cNetworkIO})
}

// ContainerBlockIO handles GET requests on /container/metric/blockio/:id?host=<hostName>
// if id (container id or name) and host is present, response blockIO metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerBlockIO(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	var cBlockIO []struct {
		BlockRead  float64
		BlockWrite float64
		ReadTime   string
	}

	for _, cm := range csm {
		cBlockIO = append(cBlockIO, struct {
			BlockRead  float64
			BlockWrite float64
			ReadTime   string
		}{BlockRead: cm.BlockRead, BlockWrite: cm.BlockWrite, ReadTime: cm.ReadTime})
	}

	ctx.JSONP(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cBlockIO})
}

// ContainerInfo handles GET requests on /container/info?host=<hostName>
// if id (container id or name) and host is present, response blockIO metric for the host
// if id (container id or name) and host is not present, response "no such container error"
func ContainerInfo(ctx *gin.Context) {
	cinfo := struct {
		Len   int
		Names []string
	}{}
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam("must", hostName); err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
		return
	}
	cinfo.Names = container.GetHostContainerInfo(hostName)
	if cinfo.Names == nil {
		ctx.JSONP(http.StatusNotFound, "stack got no container metrics")
		return
	}
	cinfo.Len = len(cinfo.Names)
	ctx.JSONP(http.StatusOK, cinfo)
}

func AddDockerhost(ctx *gin.Context) {
	host := ctx.Params.ByName("host")
	//port := ctx.DefaultQuery("host", "2375")

	if !container.IsKnownHost(host) {
		common.HostIPs = append(common.HostIPs, host)
	}

	if _, ok := container.AllHostList.Load(host); ok {
		ctx.JSONP(http.StatusNotFound, "host is already in collecting, no need to collect again")
		return
	}

	cli, err := common.InitClient(host)
	if err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
		return
	}

	go container.Monitor(cli, host)
	container.DockerCliList.Store(host, cli)
	ctx.JSONP(http.StatusOK, "successfully add")
}

func StopDockerHostCollect(ctx *gin.Context) {
	host := ctx.Params.ByName("host")

	if !container.IsKnownHost(host) {
		ctx.JSONP(http.StatusNotFound, "host does not exist, please check again")
		return
	}

	if hoststackTmp, ok := container.AllHostList.Load(host); ok {
		if dh, ok := hoststackTmp.(*container.DockerHost); ok {
			dh.StopCollect()
			time.Sleep(5 * time.Millisecond)
			if container.GetHostContainerInfo(host) == nil {
				ctx.JSONP(http.StatusOK, "successfully stopped")
				return
			}
		}
	}

	ctx.JSONP(http.StatusNotFound, "already stopped, no need to stop again")
}

var upGrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// ContainerLogs handles GET requests on /container/logs?host=<hostName>&id=<containerID>
// if id (container id or name) and host is present, response real time container log for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerLogs(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	size := ctx.DefaultQuery("size", "500")
	_, err := strconv.Atoi(size)
	if size != "all" && err != nil {
		size = "500"
	}

	logOptions := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Follow:     true,
		Details:    true,
		Tail:       size,
	}

	// upgrade http-Get to WebSocket
	ws, err := upGrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	if err := checkParam(id, hostName); err != nil {
		err = ws.WriteMessage(1, []byte(err.Error()))
		if err != nil {
			log.Printf("err ccured when write check parameter error for access container log: %v", err)
		}
		return
	}

	if cliTmp, isLoaded := container.DockerCliList.Load(hostName); isLoaded {
		if cli, ok := cliTmp.(*client.Client); ok {
			logBody, err := cli.ContainerLogs(context.Background(), id, logOptions)
			if err != nil {
				err = ws.WriteMessage(1, []byte(err.Error()))
				return
			}
			defer logBody.Close()

			// read message from ws(websocket)
			go func() {
				for {
					if _, _, err := ws.NextReader(); err != nil {
						break
					}
				}
			}()

			// write container log
			br := bufio.NewReader(logBody)
			for {
				lineBytes, err := br.ReadBytes('\n')
				if err != nil {
					break
				}
				//
				err = ws.WriteMessage(websocket.TextMessage, lineBytes[8:])
				if err != nil {
					log.Printf("err occured when write container log to websocket client: %v", err)
					return
				}
			}
		}
	} else {
		errLoad := ws.WriteMessage(1, []byte("init docker cli failed for given ip/host, please checkout the host"))
		if errLoad != nil {
			log.Printf("err occured when write load err log to websocket client: %v", errLoad)
		}
		return
	}
}

// ContainerLogs handles GET requests on "/host/mem" for localhost
/*
func HostMemInfo(ctx *gin.Context) {
	vMem, err := host.VirtualMemory()
	if err != nil {
		ctx.JSONP(http.StatusNotFound, err)
		return
	}

	hostMemInfo := struct {
		Available      uint64
		Total          uint64
		Used           uint64
		Free           uint64
		BufferAndCache uint64
		UserPercent    float64
	}{
		Total:       vMem.Total / 1024,
		Used:        vMem.Used / 1024,
		Free:        vMem.Free / 1024,
		Available:   vMem.Available / 1024,
		UserPercent: math.Trunc(vMem.UsedPercent*1e2+0.5) * 1e-2,
	}
	hostMemInfo.BufferAndCache = hostMemInfo.Available - hostMemInfo.Free
	ctx.JSONP(http.StatusOK, hostMemInfo)
}*/

func checkParam(id, hostName string) error {
	if len(id) == 0 || len(hostName) == 0 {
		return errors.New("container id/name or host must given")
	}

	isHostKnown := false
	for _, h := range common.HostIPs {
		if hostName == h {
			isHostKnown = true
		}
	}

	if !isHostKnown {
		return errors.New("nknown host, please try again")
	}
	return nil
}
