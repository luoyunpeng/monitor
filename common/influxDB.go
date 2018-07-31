package common

import (
	"log"
	"time"

	"github.com/influxdata/influxdb/client/v2"
	"github.com/luoyunpeng/monitor/container"
)

const (
	hostAddr    = "http://localhost:"
	defaultPort = "8086"
	MyDB        = "docker"
	username    = "monitor"
	password    = "iscas123"
)

var (
	influCli client.Client
)

func init() {
	var err error
	influCli, err = client.NewHTTPClient(client.HTTPConfig{
		Addr:     hostAddr + defaultPort,
		Username: username,
		Password: password,
	})
	if err != nil {
		log.Fatal(err)
	}
}

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
		go write(measurement, tags, fields, containerMetrics.ReadTimeForInfluxDB)
	}
}

func write(measurement string, tags map[string]string, files map[string]interface{}, readTime time.Time) {
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  MyDB,
		Precision: "s",
	})
	if err != nil {
		log.Fatal(err)
	}

	pt, err := client.NewPoint(measurement, tags, files, readTime)
	if err != nil {
		log.Fatal(err)
	}
	bp.AddPoint(pt)

	// Write the batch
	if err := influCli.Write(bp); err != nil {
		log.Fatal(err)
	}
}
