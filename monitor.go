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
	"runtime"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
	"github.com/luoyunpeng/monitor/mem"
	"github.com/luoyunpeng/monitor/tool"
)

var (
	host      = "tcp://ip:2375"
	dockerCli *client.Client
)

type DockerClientPool struct {
	min      int
	initSize int
}

func init() {
	var err error
	dockerCli, err = initClient()
	if err != nil {
		panic(err)
	}
}

func main() {
	router := gin.Default()
	v1 := router.Group("")

	//v1.GET("/users", getUsers)
	v1.GET("/container/stats/:id", getStatsByID)
	v1.GET("/container/logs/:id", getLog)
	v1.GET("/host/mem", getHostMemInfo)

	// By default it serves on :8080
	router.Run()
}

func initClient() (*client.Client, error) {
	var (
		err error
		cli *client.Client
	)
	if runtime.GOOS == "windows" {
		log.Println("[ monitor ]  init docker client from given host")
		cli, err = client.NewClientWithOpts(client.WithHost(host))
	} else {
		cli, err = client.NewClientWithOpts(client.FromEnv)
		log.Println("[ monitor ]  init docker client from env")
	}

	if err != nil {
		return nil, err
	}
	return cli, err
}

func getStatsByID(ctx *gin.Context) {
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

	hstats, err := tool.SetByte(respByte)
	if err != nil {
		ctx.JSON(http.StatusNotFound, err)
		return
	}
	ctx.JSON(http.StatusOK, hstats)
}

func getLog(ctx *gin.Context) {
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

func getHostMemInfo(ctx *gin.Context) {
	vMem, err := mem.VirtualMemory()
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

func keepMonitor() {
	cli, err := initClient()
	if err != nil {
		log.Println("err happen when keep moniting----", err)
		return
	}
	id := ""
	resp, err := cli.ContainerStats(context.Background(), id, false)
	if err != nil {
		log.Println("err happen when geting stats from api----", err)
		return
	}
	defer resp.Body.Close()

	respByte, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("err happen when read from resp body----", err)
		return
	}

	hstats, err := tool.SetByte(respByte)
	if err != nil {
		log.Println("err happen when read from resp body----", err)
		return
	}
	fmt.Println(hstats)
}
