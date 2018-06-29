package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/luoyunpeng/monitor/common"
	"github.com/luoyunpeng/monitor/container"
	"github.com/luoyunpeng/monitor/host"
)

var (
	dockerCli *client.Client
	hostsIPs  = []string{"localhost"}
)

func init() {
	var err error
	dockerCli, err = common.InitClient("localhost")
	if err != nil {
		panic(err)
	}

	if runtime.NumCPU() >= 4 {
		numProces := runtime.NumCPU() / 2
		runtime.GOMAXPROCS(numProces)
		fmt.Println("[ monitor ] set max processor to ", numProces)
	}
}

func main() {
	router := gin.Default()
	v1 := router.Group("")

	v1.GET("/container/stats/:id", ContainerStats)
	v1.GET("/container/metric/mem/:id", ContainerMem)
	v1.GET("/container/metric/mempercent/:id", ContainerMemPercent)
	v1.GET("/container/metric/memlimit/:id", ContainerMemLimit)
	v1.GET("/container/metric/cpu/:id", ContainerCPU)
	v1.GET("/container/metric/networkio/:id", ContainerNetworkIO)
	v1.GET("/container/metric/blockio/:id", ContainerBlockIO)
	v1.GET("/container/info", ContainerInfo)
	v1.GET("/container/logs/:id", ContainerLogs)
	v1.GET("/host/mem", HostMemInfo)

	go func() {
		for _, ip := range hostsIPs {
			if ip == "localhost" {
				go container.KeepStats(dockerCli, ip)
			} else {
				cli, err := common.InitClient(ip)
				if err != nil {
					log.Println("connect to ", ip, " err :", err)
					continue
				}
				go container.KeepStats(cli, ip)
			}
		}
	}()

	router.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		/*AllowOriginFunc: func(origin string) bool {
			return origin == "https://github.com"
		},*/
		MaxAge: 1 * time.Hour,
	}))
	// By default it serves on :8080
	router.Run()
}

func ContainerStats(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}

	hstats, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, hstats)
	/*
		resp, err := dockerCli.ContainerStats(context.Background(), id, false)
		if err != nil {
			ctx.JSON(http.StatusNotFound, err)
			return
		}
		defer resp.Body.Close()

		respByte, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			ctx.JSON(http.StatusNotFound, err)
			return
		}

		hstats, err := container.Collect(respByte)
		if err != nil {
			ctx.JSON(http.StatusNotFound, err)
			return
		}
	*/
}

func ContainerMem(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.String(http.StatusNotFound, err.Error())
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

	ctx.JSON(http.StatusOK, cMem)
}

func ContainerMemPercent(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}

	var cMemPercent struct {
		UsedPercentage  float64
		UnUsePercentage float64
		ReadTime        string
	}

	if len(csm) >= 1 {
		cMemPercent.UsedPercentage = csm[0].MemoryPercentage
		cMemPercent.UnUsePercentage = 100 - cMemPercent.UsedPercentage
		cMemPercent.ReadTime = csm[0].ReadTime
	}

	ctx.JSON(http.StatusOK, cMemPercent)
}

func ContainerMemLimit(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}

	var cMemLimit struct {
		MemoryLimit float64
		ReadTime    string
	}

	if len(csm) >= 1 {
		cMemLimit.MemoryLimit = csm[0].MemoryLimit
		cMemLimit.ReadTime = csm[0].ReadTime
	}

	ctx.JSON(http.StatusOK, cMemLimit)
}

func ContainerCPU(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.String(http.StatusNotFound, err.Error())
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

	ctx.JSON(http.StatusOK, cCPU)
}

func ContainerNetworkIO(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.String(http.StatusNotFound, err.Error())
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

	ctx.JSON(http.StatusOK, cNetworkIO)
}

func ContainerBlockIO(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam(id, hostName); err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}

	csm, err := container.GetContainerMetrics(hostName, id)
	if err != nil {
		ctx.String(http.StatusNotFound, err.Error())
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

	ctx.JSON(http.StatusOK, cBlockIO)
}

func ContainerInfo(ctx *gin.Context) {
	cinfo := struct {
		Len   int
		Names []string
	}{}
	hostName := ctx.DefaultQuery("host", "")
	if err := checkParam("must", hostName); err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}
	cinfo.Names = container.GetCInfo(hostName)
	if cinfo.Names == nil {
		ctx.JSON(http.StatusNotFound, "stack got no container metrics")
		return
	}
	cinfo.Len = len(cinfo.Names)
	ctx.JSON(http.StatusOK, cinfo)
}

func ContainerLogs(ctx *gin.Context) {
	id := ctx.Param("id")
	size := ctx.DefaultQuery("size", "500")
	_, err := strconv.Atoi(size)
	if size != "all" && err != nil {
		size = "500"
	}

	logOptions := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Since:      "",
		Until:      "",
		Follow:     false,
		Details:    false,
		Tail:       size,
	}
	bufferLogString := bytes.NewBufferString("")
	respRead, err := dockerCli.ContainerLogs(context.Background(), id, logOptions)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err.Error())
		return
	}
	defer respRead.Close()

	fileReader := bufio.NewReader(respRead)
	for {
		line, errRead := fileReader.ReadString('\n')
		if errRead == io.EOF {
			break
		}
		bufferLogString.WriteString(line[8:])
	}
	ctx.String(http.StatusOK, bufferLogString.String())
}

func HostMemInfo(ctx *gin.Context) {
	vMem, err := host.VirtualMemory()
	if err != nil {
		ctx.JSON(http.StatusNotFound, err)
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
	ctx.JSON(http.StatusOK, hostMemInfo)
}

func checkParam(id, hostName string) error {
	if len(id) == 0 || len(hostName) == 0 {
		return errors.New("container id/name or host must given")
	}

	isHostKnown := false
	for _, h := range hostsIPs {
		if hostName == h {
			isHostKnown = true
		}
	}

	if !isHostKnown {
		return errors.New("nknown host, please try again")
	}
	return nil
}
