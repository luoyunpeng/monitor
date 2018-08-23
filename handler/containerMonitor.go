package handler

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/luoyunpeng/monitor/common"
	"github.com/luoyunpeng/monitor/container"
	"github.com/luoyunpeng/monitor/host"
)

// Container's all readable metric
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

// Container's used memory
func ContainerMem(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
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
		}{Mem: cm.Memory, ReadTime: strings.Split(cm.ReadTime, " ")[1]})
	}

	ctx.JSONP(http.StatusOK, cMem)
}

// Container's memory usage percentage
func ContainerMemPercent(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
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
		cMemPercent.ReadTime = strings.Split(csm[len(csm)-1].ReadTime, " ")[1]
	}

	ctx.JSONP(http.StatusOK, cMemPercent)
}

// Container's memory limit
func ContainerMemLimit(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
		return
	}

	var cMemLimit struct {
		MemoryLimit float64
		ReadTime    string
	}

	if len(csm) >= 1 {
		cMemLimit.MemoryLimit = csm[len(csm)-1].MemoryLimit
		cMemLimit.ReadTime = strings.Split(csm[len(csm)-1].ReadTime, " ")[1]
	}

	ctx.JSONP(http.StatusOK, cMemLimit)
}

// Container's cpu usage percentage
func ContainerCPU(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
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
		}{CPU: cm.CPUPercentage, ReadTime: strings.Split(cm.ReadTime, " ")[1]})
	}

	ctx.JSONP(http.StatusOK, cCPU)
}

// Container's network TX
func ContainerNetworkIO(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
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
		}{NetworkTX: cm.NetworkTx, NetworkRX: cm.NetworkRx, ReadTime: strings.Split(cm.ReadTime, " ")[1]})
	}

	ctx.JSONP(http.StatusOK, cNetworkIO)
}

// Container's block read and write
func ContainerBlockIO(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.JSONP(http.StatusNotFound, err.Error())
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
		}{BlockRead: cm.BlockRead, BlockWrite: cm.BlockWrite, ReadTime: strings.Split(cm.ReadTime, " ")[1]})
	}

	ctx.JSONP(http.StatusOK, cBlockIO)
}

// Container's basic info
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

var upGrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Container's real time log
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

	//upgrade http-Get to WebSocket
	ws, err := upGrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	if err := checkParam(id, hostName); err != nil {
		err = ws.WriteMessage(1, []byte(err.Error()))
		if err != nil {
			fmt.Printf("err ccured when write check parameter error for access container log: %v", err)
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

			//read message from ws(websocket)
			go func() {
				for {
					if _, _, err := ws.NextReader(); err != nil {
						break
					}
				}
			}()

			//write container log
			br := bufio.NewReader(logBody)
			for {
				lineBytes, err := br.ReadBytes('\n')
				if err != nil {
					break
				}
				//
				err = ws.WriteMessage(websocket.TextMessage, lineBytes[8:])
				if err != nil {
					fmt.Printf("err occured when write container log to websocket client: %v", err)
					return
				}
			}
		}
	} else {
		errLoad := ws.WriteMessage(1, []byte("init docker cli failed for given ip/host, please checkout the host"))
		if errLoad != nil {
			fmt.Printf("err occured when write load err log to websocket client: %v", errLoad)
		}
		return
	}
}

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
}

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
