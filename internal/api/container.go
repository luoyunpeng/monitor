package api

import (
	"bufio"
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/archive"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/luoyunpeng/monitor/internal/config"
	"github.com/luoyunpeng/monitor/internal/models"
	"github.com/luoyunpeng/monitor/internal/monitor"
	"github.com/luoyunpeng/monitor/internal/util"
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
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	csm, err := models.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
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
	ctx.JSONP(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cMem})
}

// ContainerMemPercent handles GET requests on /container/metric/mempercent/:id?host=<hostName>
// if id (container id or name) and host is present, response memory usage metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerMemPercent(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	csm, err := models.GetContainerMetrics(hostName, id)
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
		cMemPercent.UnUsePercentage = util.Round(100-cMemPercent.UsedPercentage, 3)
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
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	csm, err := models.GetContainerMetrics(hostName, id)
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
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	csm, err := models.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
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

	ctx.JSONP(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cCPU})
}

// ContainerNetworkIO handles GET requests on /container/metric/networkio/:id?host=<hostName>
// if id (container id or name) and host is present, response networkIO metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerNetworkIO(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	csm, err := models.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
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

	ctx.JSONP(http.StatusOK, RepMetric{Status: 1, StatusCode: http.StatusOK, Msg: "", Metric: cNetworkIO})
}

// ContainerBlockIO handles GET requests on /container/metric/blockio/:id?host=<hostName>
// if id (container id or name) and host is present, response blockIO metric for the container
// if id (container id or name) and host is not present, response "no such container error"
func ContainerBlockIO(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	csm, err := models.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: err.Error(), Metric: nil})
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
		ctx.JSONP(http.StatusNotFound, "host is already in collecting, no need to collect again")
		return
	}

	cli, err := monitor.InitClient(host)
	if err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
		return
	}

	models.StoppedDockerHost.Delete(host)
	if config.MonitorInfo.DockerHostIndex(host) == -1 {
		config.MonitorInfo.AddHost(host)
	}
	go monitor.Monitor(cli, host, config.MonitorInfo.Logger)
	ctx.JSONP(http.StatusOK, "successfully add")
}

// StopDockerHostCollect default /host/stop/:host?rm=0
func StopDockerHostCollect(ctx *gin.Context) {
	var rmStop bool
	host := ctx.Params.ByName("host")
	rm := ctx.DefaultQuery("rm", "0")
	if rm == "1" {
		rmStop = true
	}

	if config.MonitorInfo.DockerHostIndex(host) == -1 {
		ctx.JSONP(http.StatusNotFound, "host does not exist, please check again")
		return
	}

	if dh, err := models.GetDockerHost(host); err == nil {
		dh.StopCollect(rmStop)
		time.Sleep(1 * time.Millisecond)
		if models.GetHostContainerInfo(host) == nil {
			ctx.JSONP(http.StatusOK, "successfully stopped")
			return
		}
	}

	ctx.JSONP(http.StatusNotFound, "already stopped, no need to stop again")
}

// DownDockerHostInfo
func DownDockerHostInfo(ctx *gin.Context) {
	ips := models.AllStoppedDHIP()

	ctx.JSONP(http.StatusOK, struct {
		Len int
		IPS []string
	}{Len: len(ips), IPS: ips})
}

// AllDockerHostInfo
func AllDockerHostInfo(ctx *gin.Context) {
	ctx.JSONP(http.StatusOK, config.MonitorInfo.GetHosts())
}

// ContainerSliceCapDebug
func ContainerSliceCapDebug(ctx *gin.Context) {
	host := ctx.Params.ByName("host")

	if config.MonitorInfo.DockerHostIndex(host) == -1 {
		ctx.JSONP(http.StatusNotFound, "host does not exist, please check again")
		return
	}

	if dh, err := models.GetDockerHost(host); err == nil {
		ctx.JSONP(http.StatusOK, dh.Length())
		return
	}
	ctx.JSONP(http.StatusNotFound, "stopped host")
}

// CopyAcrossContainer for testing will rm
func CopyAcrossContainer(ctx *gin.Context) {
	fileName := ctx.Params.ByName("file")
	destHost := ctx.DefaultQuery("host", "")
	if fileName == "" || destHost == "" {
		ctx.JSON(http.StatusNotFound, "file name/destHost must given")
		return
	}
	srcHost := "192.168.100.177"

	srcContainer := "testcp1"
	destContainer := "testcp"

	srcPath := "/opt/"
	destPath := srcPath
	srcPath += fileName
	c := context.Background()

	srcDH, err := models.GetDockerHost(srcHost)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}
	destDH, err := models.GetDockerHost(destHost)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}

	content, name, err := srcDH.CopyFromContainer(c, srcContainer, srcPath)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}

	err = destDH.CopyToContainer(c, content, destContainer, destPath, name)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}

	ctx.JSON(http.StatusOK, "across containers copy ok")
}

// CopyAcrossContainer_order backup copying
func CopyAcrossContainer_order(ctx *gin.Context) {
	srcOrderId := ctx.DefaultQuery("srcOrder", "")
	destOrderId := ctx.DefaultQuery("destOrder", "")
	if srcOrderId == "" || destOrderId == "" {
		ctx.JSON(http.StatusNotFound, "src and dest orderId must given")
		return
	}

	srcOrderInfo, err := monitor.QueryOrder(srcOrderId)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}
	destOrderInfo, err := monitor.QueryOrder(destOrderId)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}

	err = models.CheckOrderInfo(srcOrderInfo, destOrderInfo)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}

	ctx.JSON(http.StatusOK, "across containers copy ok")
}

// CopyFromContainer copies file from container to local
func CopyFromContainer(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	srcPath := ctx.DefaultQuery("srcPath", "/opt/repchain/RepChainDB")
	if errInfo := checkParam(id, hostName); errInfo != "" {
		ctx.JSONP(http.StatusOK, RepMetric{Status: 0, StatusCode: http.StatusInternalServerError, Msg: errInfo, Metric: nil})
		return
	}

	srcDH, err := models.GetDockerHost(hostName)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}

	baseName := filepath.Base(srcPath)
	content, _, err := srcDH.CopyFromContainer(context.Background(), id, srcPath)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}

	hashDir := util.ComputeHmac256(id, hostName)[:18]
	err = os.MkdirAll("./"+hashDir, os.ModeDir)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}
	dstPath := "./" + hashDir + "/"
	srcInfo := archive.CopyInfo{
		Path:       srcPath,
		Exists:     true,
		IsDir:      true,
		RebaseName: "",
	}
	err = archive.CopyTo(content, srcInfo, dstPath)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}

	if !util.Exists(dstPath + baseName) {
		ctx.JSON(http.StatusNotFound, "copy from container failed")
		return
	}

	err = util.Zip(dstPath+baseName, dstPath+baseName+".zip")
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}
	defer os.RemoveAll(dstPath)

	ctx.File(dstPath + baseName + ".zip")
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
			log.Printf("err ccured when write check parameter error for access container log: %v", err)
		}
		return
	}

	dh, err := models.GetDockerHost(hostName)
	if err == nil && dh.IsValid() {
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
				log.Printf("err occured when write container log to websocket client: %v", err)
				return
			}
		}
	}

	errLoad := ws.WriteMessage(1, []byte("init docker cli failed for given ip/host, please checkout the ip/host"))
	if errLoad != nil {
		log.Printf("err occured when write load err log to websocket client: %v", errLoad)
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
