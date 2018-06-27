package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
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
}

func main() {
	router := gin.Default()
	v1 := router.Group("")

	v1.GET("/container/stats/:id", ContainerStats)
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

	// By default it serves on :8080
	router.Run()
}

func ContainerStats(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	host := ctx.DefaultQuery("host", "")
	if len(id) == 0 || len(host) == 0 {
		ctx.JSON(http.StatusNotFound, "container id/name or host must given")
		return
	}

	isHostKnown := false
	for _, h := range hostsIPs {
		if host == h {
			isHostKnown = true
		}
	}

	if !isHostKnown {
		ctx.JSON(http.StatusNotFound, "nknown host, please try again")
	}

	hstats, err := container.GetContainerMetrics(host, id)
	if err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}
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
	ctx.JSON(http.StatusOK, hstats)
}

func ContainerInfo(ctx *gin.Context) {
	cinfo := struct {
		Len   int
		Names []string
	}{}
	cinfo.Names = container.GetCInfo("")
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

func ContainersID() ([]string, error) {
	listOpt := types.ContainerListOptions{
		Quiet: true,
	}
	containers, err := dockerCli.ContainerList(context.Background(), listOpt)
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(containers))
	for _, c := range containers {
		ids = append(ids, c.ID[:12])
	}

	return ids, nil
}
