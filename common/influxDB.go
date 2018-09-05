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
	influCli   client.Client
	MetricChan chan Metric
)

// Metric is basic unit that insert into influxDB
type Metric struct {
	Measurement string
	Tags        map[string]string
	Fields      map[string]interface{}
	ReadTime    time.Time
}

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

	MetricChan = make(chan Metric, 8)
}

// Write giving tag kv and files kv to giving measurement
func Write() {
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  myDB,
		Precision: "s",
	})
	if err != nil {
		log.Printf("err happen when new  batch points: %v", err)
	}

	for m := range MetricChan {
		pt, err := client.NewPoint(m.Measurement, m.Tags, m.Fields, m.ReadTime)
		if err != nil {
			log.Printf("err happen when new point: %v", err)
		}
		bp.AddPoint(pt)

		// Write the batch
		if err := influCli.Write(bp); err != nil {
			log.Printf("err happen when write the batch point: %v", err)
		}
	}
}
