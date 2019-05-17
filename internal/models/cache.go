package models

import (
	"errors"
	"sync"

	"github.com/luoyunpeng/monitor/internal/config"
)

var (
	// Each container can store at most 15 stats record in individual container stack
	// Each Host has at least one container stack, we use Docker host to store the container stacks
	// AllHostList stores every host's Docker host, the key is host ip address
	DockerHostCache sync.Map
	// All stopped docker host
	StoppedDockerHost sync.Map
)

// GetContainerMetrics return container stats
func GetContainerMetrics(host, id string) ([]ParsedConatinerMetric, error) {
	if hoststackTmp, ok := DockerHostCache.Load(host); ok {
		if dh, ok := hoststackTmp.(*DockerHost); ok {
			for _, containerStack := range dh.GetContainerStats() {
				if containerStack.ID == id || (len(id) >= 12 && containerStack.ID == id[:12]) || containerStack.ContainerName == id {
					return containerStack.Read(config.MonitorInfo.CacheNum), nil
				}
			}
			return nil, errors.New("given container name or id is unknown, or container is not running")
		}
	}

	return nil, errors.New("given host " + host + " is not loaded")
}

func GetDockerHost(ip string) (*DockerHost, error) {
	if hoststackTmp, ok := DockerHostCache.Load(ip); ok {
		if dh, ok := hoststackTmp.(*DockerHost); ok {
			return dh, nil
		}
	}
	return nil, errors.New("no such host")
}
