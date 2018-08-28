package common

import (
	"log"
	"time"

	"github.com/influxdata/influxdb/client/v2"
)

const (
	hostAddr    = "http://localhost:"
	defaultPort = "8086"
	myDB        = "docker"
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

// Write giving tag kv and files kv to giving measurement
func Write(measurement string, tags map[string]string, files map[string]interface{}, readTime time.Time) {
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  myDB,
		Precision: "s",
	})
	if err != nil {
		log.Println("err happen when new  batch points: ", err)
	}

	pt, err := client.NewPoint(measurement, tags, files, readTime)
	if err != nil {
		log.Println("err happen when new point: ", err)
	}
	bp.AddPoint(pt)

	// Write the batch
	if err := influCli.Write(bp); err != nil {
		log.Println("err happen when write the batch point: ", err)
	}
}
