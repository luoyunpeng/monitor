package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/luoyunpeng/monitor/internal/config"
	"github.com/luoyunpeng/monitor/internal/models"
	"github.com/luoyunpeng/monitor/internal/monitor"
	"github.com/luoyunpeng/monitor/internal/util"
)

// RepMetric represents api get response, for easy, wrap status code
// TODO: based on needed, remove status code,
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
	hostName := ctx.Query("host")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSON(http.StatusNotFound, errInfo)
		return
	}

	hstats, err := models.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, hstats)
}

// ContainerMem handles GET requests on /container/metric/mem/:id?host=<hostName>
// if id (container id or name) and host is present, response mem metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerMem(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	csm, err := models.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	cMem := make([]struct {
		Mem      float64
		ReadTime string
	}, 0, config.MonitorInfo.CacheNum)

	for _, cm := range csm {
		cMem = append(cMem, struct {
			Mem      float64
			ReadTime string
		}{Mem: cm.Memory, ReadTime: cm.ReadTime})
	}
	ctx.JSON(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cMem})
}

// ContainerMemPercent handles GET requests on /container/metric/mempercent/:id?host=<hostName>
// if id (container id or name) and host is present, response memory usage metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerMemPercent(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	csm, err := models.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	var cMemPercent struct {
		UsedPercentage  float64
		UnUsePercentage float64
		ReadTime        string
	}

	if len(csm) >= 1 {
		cMemPercent.UsedPercentage = csm[len(csm)-1].MemoryPercentage
		cMemPercent.UnUsePercentage = util.Round(100-cMemPercent.UsedPercentage, 3)
		cMemPercent.ReadTime = csm[len(csm)-1].ReadTime
	}

	ctx.JSON(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cMemPercent})
}

// ContainerMemLimit handles GET requests on /container/metric/memlimit/:id?host=<hostName>
// if id (container id or name) and host is present, response memory limit metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerMemLimit(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	csm, err := models.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
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

	ctx.JSON(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cMemLimit})
}

// ContainerCPU handles GET requests on /container/metric/cpu/:id?host=<hostName>
// if id (container id or name) and host is present, response cpu usage metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerCPU(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	csm, err := models.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	cCPU := make([]struct {
		CPU      float64
		ReadTime string
	}, 0, config.MonitorInfo.CacheNum)

	for _, cm := range csm {
		cCPU = append(cCPU, struct {
			CPU      float64
			ReadTime string
		}{CPU: cm.CPUPercentage, ReadTime: cm.ReadTime})
	}

	ctx.JSON(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cCPU})
}

// ContainerNetworkIO handles GET requests on /container/metric/networkio/:id?host=<hostName>
// if id (container id or name) and host is present, response networkIO metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerNetworkIO(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	csm, err := models.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	cNetworkIO := make([]struct {
		NetworkTX float64
		NetworkRX float64
		ReadTime  string
	}, 0, config.MonitorInfo.CacheNum)

	for _, cm := range csm {
		cNetworkIO = append(cNetworkIO, struct {
			NetworkTX float64
			NetworkRX float64
			ReadTime  string
		}{NetworkTX: cm.NetworkTx, NetworkRX: cm.NetworkRx, ReadTime: cm.ReadTime})
	}

	ctx.JSON(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cNetworkIO})
}

// ContainerBlockIO handles GET requests on /container/metric/blockio/:id?host=<hostName>
// if id (container id or name) and host is present, response blockIO metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerBlockIO(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	csm, err := models.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
		return
	}

	cBlockIO := make([]struct {
		BlockRead  float64
		BlockWrite float64
		ReadTime   string
	}, 0, config.MonitorInfo.CacheNum)

	for _, cm := range csm {
		cBlockIO = append(cBlockIO, struct {
			BlockRead  float64
			BlockWrite float64
			ReadTime   string
		}{BlockRead: cm.BlockRead, BlockWrite: cm.BlockWrite, ReadTime: cm.ReadTime})
	}

	ctx.JSON(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cBlockIO})
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
	if errInfo := checkParam("must", hostName); errInfo != "" {
		ctx.JSON(http.StatusNotFound, errInfo)
		return
	}
	cinfo.Names = models.GetHostContainerInfo(hostName)
	if cinfo.Names == nil {
		ctx.JSON(http.StatusNotFound, "stack got no container metrics")
		return
	}
	cinfo.Len = len(cinfo.Names)
	ctx.JSONP(http.StatusOK, cinfo)
}

// AddDockerhost add host that running docker with exposing port 2375 to the monitor list
func AddDockerhost(ctx *gin.Context) {
	host := ctx.Params.ByName("host")
	//port := ctx.DefaultQuery("host", "2375")

	// if host already in monitor list, return
	if _, ok := models.DockerHostCache.Load(host); ok {
		ctx.JSON(http.StatusNotFound, "host is already in collecting, no need to collect again")
		return
	}

	cli, err := monitor.InitClient(host)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}

	models.StoppedDockerHost.Delete(host)
	if config.MonitorInfo.DockerHostIndex(host) == -1 {
		config.MonitorInfo.AddHost(host)
	}
	go monitor.Monitor(cli, host, config.MonitorInfo.Logger)
	ctx.JSON(http.StatusOK, "successfully add")
}

// StopDockerHostCollect stop docker host /host/:host
func StopDockerHostCollect(ctx *gin.Context) {
	host := ctx.Params.ByName("host")

	if config.MonitorInfo.DockerHostIndex(host) == -1 {
		ctx.JSON(http.StatusNotFound, "host does not exist, please check again")
		return
	}

	if dh, err := models.GetDockerHost(host); err == nil {
		dh.StopCollect(false)
		time.Sleep(1 * time.Millisecond)
		if models.GetHostContainerInfo(host) == nil {
			ctx.JSON(http.StatusOK, "successfully stopped")
			return
		}
	}

	ctx.JSON(http.StatusNotFound, "already stopped, no need to stop again")
}

// DeleteDockerHost delete docker host  /host/:host
func DeleteDockerHost(ctx *gin.Context) {
	host := ctx.Params.ByName("host")

	if config.MonitorInfo.DockerHostIndex(host) == -1 {
		ctx.JSON(http.StatusNotFound, "host does not exist, please check again")
		return
	}

	if dh, err := models.GetDockerHost(host); err == nil {
		dh.StopCollect(true)
		time.Sleep(1 * time.Millisecond)
		if models.GetHostContainerInfo(host) == nil {
			ctx.JSON(http.StatusOK, "successfully deleted")
			return
		}
	}

	ctx.JSON(http.StatusNotFound, "already deleted, no need to delete again")
}

// DownDockerHostInfo return the host info in stop-cache
func DownDockerHostInfo(ctx *gin.Context) {
	ips := models.AllStoppedDHIP()

	ctx.JSON(http.StatusOK, struct {
		Len int
		IPS []string
	}{Len: len(ips), IPS: ips})
}

// AllDockerHostInfo return all host in configure file
func AllDockerHostInfo(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, config.MonitorInfo.GetHosts())
}

// ContainerSliceCapDebug is the slice cap, just for debug
func ContainerSliceCapDebug(ctx *gin.Context) {
	host := ctx.Params.ByName("host")

	if config.MonitorInfo.DockerHostIndex(host) == -1 {
		ctx.JSON(http.StatusNotFound, "host does not exist, please check again")
		return
	}

	if dh, err := models.GetDockerHost(host); err == nil {
		ctx.JSON(http.StatusOK, dh.Length())
		return
	}
	ctx.JSON(http.StatusNotFound, "stopped host")
}

// CopyAcrossContainer backup copying by order
func CopyAcrossContainer(ctx *gin.Context) {
	srcOrderId := ctx.DefaultQuery("srcOrder", "")
	destOrderId := ctx.DefaultQuery("destOrder", "")
	if srcOrderId == "" || destOrderId == "" {
		ctx.JSON(http.StatusBadRequest, "src and dest orderId must given")
		return
	}

	srcOrderInfo, err := monitor.QueryOrder(srcOrderId)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, err.Error())
		return
	}
	destOrderInfo, err := monitor.QueryOrder(destOrderId)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, err.Error())
		return
	}

	err = models.CheckOrderInfo(srcOrderInfo, destOrderInfo)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, err.Error())
		return
	}

	ctx.JSON(http.StatusOK, "across containers copy ok")
}

// PauseService pauses all container in the order id
func PauseService(ctx *gin.Context) {
	srcOrderId := ctx.DefaultQuery("order", "")
	if srcOrderId == "" {
		ctx.JSON(http.StatusBadRequest, "orderId must apply")
		return
	}

	orderInfo, err := monitor.QueryOrder(srcOrderId)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, err.Error())
		return
	}
	c := context.Background()
	for _, info := range orderInfo {
		dh, err := models.GetDockerHost(info.IpAddr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, err.Error())
			return
		}
		err = dh.Cli.ContainerPause(c, info.ContainerID)
		if err != nil {
			// print it, do not handle it
			dh.Logger.Printf("[%s] container pause error: %v", dh.GetIP(), err)
			continue
		}
		dh.Logger.Printf("[%s] container: %s", dh.GetIP(), info.ContainerID)
	}

	ctx.JSON(http.StatusOK, "service containers pause ok")
}

// DownloadFromContainer download file or directory from container
func DownloadFromContainer(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	srcPath := ctx.DefaultQuery("srcPath", "RepChainDB")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSON(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusBadRequest, Msg: errInfo, Metric: nil})
		return
	}
	baseName := srcPath
	srcPath = "/opt/repchain/" + srcPath
	srcDH, err := models.GetDockerHost(hostName)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, err.Error())
		return
	}

	srcStat, err := srcDH.Cli.ContainerStatPath(ctx, id, srcPath)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, err.Error())
		return
	}
	content, _, err := srcDH.CopyFromContainer(context.Background(), id, srcPath)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, err.Error())
		return
	}
	defer content.Close()

	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Header("Content-Disposition", "attachment; filename="+baseName+".tar")
	ctx.Header("Content-Type", "application/octet-stream")
	ctx.Header("Accept-Length", fmt.Sprintf("%d", srcStat.Size))

	var buf [64]byte
	_, err = io.CopyBuffer(ctx.Writer, content, buf[:])
	if err != nil {
		ctx.JSON(http.StatusBadRequest, err.Error())
	}
}

var upGrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// ContainerLogs handles GET requests on /container/logs/id?host=<hostName>&size=<logSize>
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

	if errInfo := checkParam(id, hostName); errInfo != "" {
		err = ws.WriteMessage(1, util.Str2bytes(errInfo))
		if err != nil {
			log.Printf("err occurred when write check parameter error for access container log: %v", err)
		}
		return
	}

	if dh, err := models.GetDockerHost(hostName); err == nil && dh.IsValid() {
		logBody, err := dh.Cli.ContainerLogs(context.Background(), id, logOptions)
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
				log.Printf("err occurred when write container log to websocket client: %v", err)
				return
			}
		}
	}

	errLoad := ws.WriteMessage(1, []byte("init docker cli failed for given ip/host, please checkout the ip/host"))
	if errLoad != nil {
		log.Printf("err occurred when write load err log to websocket client: %v", errLoad)
	}
}

// ContainerConsole handles GET requests on /container/console/id?host=<hostName>&cmd=</bin/bash>
func ContainerConsole(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	cmd := ctx.DefaultQuery("cmd", "/bin/bash")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	dh, err := models.GetDockerHost(hostName)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}

	if !dh.IsContainerRunning(id) {
		ctx.JSON(http.StatusNotFound, fmt.Errorf("[%s] %s is not running please check", dh.GetIP(), id))
		return
	}

	// upgrade http-Get to WebSocket
	ws, err := upGrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}
	defer ws.Close()

	err = dh.ContainerConsole(context.Background(), ws, id, cmd)
	if err != nil {
		log.Println("[container console err]", err)
		ctx.JSON(http.StatusNotFound, err.Error())
	}
}

// ContainerTtyResize handles GET requests on /container/ttyresize/id?host=<hostName or ip addr>
func ContainerTtyResize(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	dh, err := models.GetDockerHost(hostName)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}
	<-dh.Done
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

// checkParam
func checkParam(id, hostName string) string {
	if len(id) == 0 || len(hostName) == 0 {
		return "container id/name or host must given"
	}

	isHostKnown := false
	for _, h := range config.MonitorInfo.GetHosts() {
		if hostName == h {
			isHostKnown = true
		}
	}

	if !isHostKnown {
		return "nknown host, please try again"
	}
	return ""
}
