package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
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

	// By default it serves on :8080
	initClient()
	router.Run()
}

func initClient() (*client.Client, error) {
	var (
		err error
		cli *client.Client
	)
	if runtime.GOOS == "windows" {
		log.Println("init docker client from given host")
		cli, err = client.NewClientWithOpts(client.WithHost(host))
	} else {
		cli, err = client.NewClientWithOpts(client.FromEnv)
		log.Println("init docker client from Env")
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
