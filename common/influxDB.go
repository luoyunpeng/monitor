package common

import (
	"log"
	"time"

	"github.com/influxdata/influxdb/client/v2"
)

const (
	hostAddr    = "http://localhost:"
	defaultPort = "8086"
	MyDB        = "docker"
	username    = "monitor"
	password    = "iscas123"
)

var (
	InfluCli client.Client
)

func init() {
	var err error
	InfluCli, err = client.NewHTTPClient(client.HTTPConfig{
		Addr:     hostAddr + defaultPort,
		Username: username,
		Password: password,
	})
	if err != nil {
		log.Fatal(err)
	}
}

func Write(measurement string, tags map[string]string, files map[string]interface{}, readTime time.Time) {
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
	if err := InfluCli.Write(bp); err != nil {
		log.Fatal(err)
	}
}
