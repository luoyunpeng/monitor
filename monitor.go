package main

import (
"bufio"
"bytes"
"context"
"fmt"
"io"
"io/ioutil"
"log"
"math"
"net/http"










"github.com/docker/docker/api/types"
"github.com/docker/docker/client"
"github.com/gin-gonic/gin"
"github.com/luoyunpeng/monitor/common"
"github.com/luoyunpeng/monitor/container"
"github.com/luoyunpeng/monitor/host"
"github.com/pkg/errors"

)

var (
	hostURL   = "tcp://ip:2375"
	dockerCli *client.Client
)

//change to sync.Pool ??
type DockerClientPool struct {
	min      int
	initSize int
}

func init() {
	var err error
	dockerCli, err = common.InitClient(hostURL)
	if err != nil {
		panic(err)
	}
}

func main() {
	router := gin.Default()
	v1 := router.Group("")

	v1.GET("/container/stats/:id", ContainerStats)
	v1.GET("/container/logs/:id", ContainerLogs)
	v1.GET("/host/mem", HostMemInfo)

	cli, err := common.InitClient(hostURL)
	if err != nil {
		panic(err)
	}
	go container.KeepStats(cli)

	// By default it serves on :8080
	router.Run()
}

func ContainerStats(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	if len(id) == 0 {
		ctx.JSON(http.StatusNotFound, "container id or name must given")
		return
	}

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
	ctx.JSON(http.StatusOK, hstats)
}

func ContainerLogs(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	size := ctx.Params.ByName("size")
	if size == "" {
		size = "all"
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
		ctx.JSON(http.StatusNotFound, err)
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

func internalStats(ids []string) ([]container.HumanizeStats, error) {
	if len(ids) == 0 {
		log.Println("container id or name must given")
		return nil, errors.New("container id or name must given")
	}

	for _, id := range ids {
		go func(id string) {
			resp, err := dockerCli.ContainerStats(context.Background(), id, false)
			if err != nil {

			}
			defer resp.Body.Close()

			respByte, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Println(err)
				return
			}

			hstats, err := container.Collect(respByte)
			if err != nil {
				log.Println(err)
				return
			}
			fmt.Println(hstats)
		}(id)
	}
	return nil, nil
}
