package container

import (
	"fmt"
	"log"
	"strconv"
	"testing"
	"time"
)

var (
	p = ParsedConatinerMetrics{ReadTime: "now"}
)

func BenchmarkNewContainerMStack(b *testing.B) {
	var demo *SingalContainerMetricStack

	for i := 1; i <= b.N; i++ {
		demo = NewContainerMStack("test", "")
	}
	ignore(*demo)
}

func ignore(s SingalContainerMetricStack) {
}

func BenchmarkContainerMetricStack_Put(b *testing.B) {
	cms := NewContainerMStack("test", "")

	for i := 1; i <= b.N; i++ {
		cms.Put(p)
	}

	log.Println(len(cms.ReadAbleMetrics), cap(cms.ReadAbleMetrics))
}

func TestContainerMPut(b *testing.T) {
	cms := NewContainerMStack("test", "")
	for i := 1; i <= 34; i++ {
		cms.Put(ParsedConatinerMetrics{ReadTime: "" + strconv.Itoa(i)})
		time.Sleep(time.Millisecond * 100)
	}

	time.Sleep(2)

	for index, p := range cms.Read(15) {
		time.Sleep(time.Second)
		fmt.Println(index, " read time: ", p.ReadTime)
		if p.ReadTime == "34" {
			break
		}
	}
	log.Println("end ***** ", len(cms.ReadAbleMetrics), cap(cms.ReadAbleMetrics))
}

func BenchmarkContainerMetricStack_Read(b *testing.B) {
	cms := NewContainerMStack("test", "")
	cms.Put(p)

	for i := 1; i <= b.N; i++ {
		cms.Read(15)

	}

	log.Println(len(cms.ReadAbleMetrics), cap(cms.ReadAbleMetrics))
}

func BenchmarkContainerMetricStack_GetLatestMemory(b *testing.B) {
	cms := NewContainerMStack("test", "")
	cms.Put(p)

	for i := 1; i <= b.N; i++ {
		if cms.GetLatestMemory() != 0 {
			log.Fatal("last mem is not right")
		}
	}

	log.Println(len(cms.ReadAbleMetrics), cap(cms.ReadAbleMetrics))
}

func BenchmarkContainerMetricStack_Length(b *testing.B) {
	cms := NewContainerMStack("test", "")
	cms.Put(p)

	for i := 1; i <= b.N; i++ {
		if len(cms.ReadAbleMetrics) != DefaultReadLength {
			log.Fatal("last mem is not right")
		}
	}

	log.Println(len(cms.ReadAbleMetrics), cap(cms.ReadAbleMetrics))
}

func BenchmarkNewHostContainerMetricStack(b *testing.B) {

	for i := 1; i <= b.N; i++ {
		NewHostContainerMetricStack("test=host")
	}
}

func BenchmarkHostContainerMetricStack_Add(b *testing.B) {
	hostsm := NewHostContainerMetricStack("testhost")

	for i := 1; i <= b.N; i++ {
		cms := NewContainerMStack("test"+strconv.Itoa(i), "")
		hostsm.Add(cms)
	}
}

func BenchmarkHostContainerMetricStack_Remove(b *testing.B) {
	hostsm := NewHostContainerMetricStack("test=host")

	num := b.N
	for i := 1; i <= num; i++ {
		cms := NewContainerMStack("test", "")
		cms.ContainerName += strconv.Itoa(i)
		hostsm.Add(cms)
	}

	for i := 1; i <= num; i++ {
		hostsm.Remove("test" + strconv.Itoa(i))
	}
}

func BenchmarkTimeFormat(b *testing.B) {
	now := time.Now()

	for i := 1; i <= b.N; i++ {
		now.Format("15:04:05")
	}
}

func BenchmarkAppendTimeFormat(b *testing.B) {
	now := time.Now()
	var buf [8]byte
	b1 := buf[:0]

	for i := 1; i <= b.N; i++ {
		now.AppendFormat(b1[:0], "15:04:05")
	}
	fmt.Println(len(string(buf[:])), string(buf[:]))
}
