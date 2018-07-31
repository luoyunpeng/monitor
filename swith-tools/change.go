package swith_tools

import (
	"github.com/luoyunpeng/monitor/common"
	"github.com/luoyunpeng/monitor/container"
)

func WriteToInfluxDB(host, containerName string, containerMetrics *container.ParsedConatinerMetrics) {
	var fields map[string]interface{}
	measurements := []string{"cpu", "mem", "networkTX", "networkRX", "blockRead", "blockWrite"}
	tags := map[string]string{
		"host": host,
		"name": containerName,
	}

	for _, measurement := range measurements {
		switch measurement {
		case "cpu":
			fields = map[string]interface{}{
				measurement: containerMetrics.CPUPercentage,
			}
		case "mem":
			fields = map[string]interface{}{
				measurement: containerMetrics.Memory,
			}
		case "networkTX":
			fields = map[string]interface{}{
				measurement: containerMetrics.NetworkTx,
			}
		case "networkRX":
			fields = map[string]interface{}{
				measurement: containerMetrics.NetworkRx,
			}
		case "blockRead":
			fields = map[string]interface{}{
				measurement: containerMetrics.BlockRead,
			}
		case "blockWrite":
			fields = map[string]interface{}{
				measurement: containerMetrics.BlockWrite,
			}
		}
		go common.Write(measurement, tags, fields, containerMetrics.ReadTimeForInfluxDB)
	}
}
