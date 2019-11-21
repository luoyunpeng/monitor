package server

import (
	"github.com/gin-gonic/gin"
	"github.com/luoyunpeng/monitor/internal/api"
)

func registerRoutes(app *gin.Engine) {
	// JSON-REST API Version 1
	v1 := app.Group("")

	{
		v1.GET("/container/stats/:id", api.ContainerStats)
		v1.GET("/container/metric/mem/:id", api.ContainerMem)
		v1.GET("/container/metric/mempercent/:id", api.ContainerMemPercent)
		v1.GET("/container/metric/memlimit/:id", api.ContainerMemLimit)
		v1.GET("/container/metric/cpu/:id", api.ContainerCPU)
		v1.GET("/container/metric/networkio/:id", api.ContainerNetworkIO)
		v1.GET("/container/metric/blockio/:id", api.ContainerBlockIO)
		v1.GET("/container/info", api.ContainerInfo)

		v1.GET("/container/logs/:id", api.ContainerLogs)
		v1.GET("/container/console/:id", api.ContainerConsole)

		v1.POST("/host/:host", api.AddDockerhost)
		v1.PUT("/host/:host/stop", api.StopDockerHostCollect)
		v1.DELETE("/host/:host", api.DeleteDockerHost)
		v1.GET("/host/down/", api.DownDockerHostInfo)
		v1.GET("/container/debug/slicecap/:host", api.ContainerSliceCapDebug)
		v1.GET("/host/all", api.AllDockerHostInfo)

		// v1.GET("/container/across/", api.CopyAcrossContainer)
		v1.GET("/container/download/:id", api.DownloadFromContainer)
	}
}
