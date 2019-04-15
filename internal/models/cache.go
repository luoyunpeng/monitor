package models

import (
	"errors"
	"sync"

	"github.com/luoyunpeng/monitor/internal/conf"
)

var (
	// Each container can store at most 15 stats record in individual container stack
	// Each Host has at least one container stack, we use Docker host to store the container stacks
	// AllHostList stores every host's Docker host, the key is host ip address
	Cache_AllHostList sync.Map
	// All stopped docker host
	Cache_StoppedDocker sync.Map
)

// GetContainerMetrics return container stats
func GetContainerMetrics(host, id string) ([]ParsedConatinerMetric, error) {
	if hoststackTmp, ok := Cache_AllHostList.Load(host); ok {
		if dh, ok := hoststackTmp.(*DockerHost); ok {
			for _, containerStack := range dh.GetContainerStats() {
				if containerStack.ID == id || (len(id) >= 12 && containerStack.ID == id[:12]) || containerStack.ContainerName == id {
					return containerStack.Read(conf.DefaultReadLength), nil
				}
			}
			return nil, errors.New("given container name or id is unknown, or container is not running")
		}
	}

	return nil, errors.New("given host " + host + " is not loaded")
}
