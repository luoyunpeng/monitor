package models

import (
	"errors"
	"sync"

	"github.com/luoyunpeng/monitor/internal/config"
)

var (
	// DockerHostCache cache docker host info the key is host ip address
	// Each container can store at most 15 stats record in individual container stack
	// Each Host has at least one container stack, we use Docker host to store the container stacks
	DockerHostCache sync.Map
	// StoppedDockerHost cache all stopped docker host
	StoppedDockerHost sync.Map
)

// GetContainerMetrics return container stats
func GetContainerMetrics(host, id string) ([]ParsedConatinerMetric, error) {
	if dh, err := GetDockerHost(host); err == nil {
		for _, containerStack := range dh.GetContainerStats() {
			if containerStack.ID == id || (len(id) >= 12 && containerStack.ID == id[:12]) || containerStack.ContainerName == id {
				return containerStack.Read(config.MonitorInfo.CacheNum), nil
			}
		}
		return nil, errors.New("given container name or id is unknown, or container is not running")
	}

	return nil, errors.New("given host " + host + " is not loaded")
}

// GetDockerHost get DockerHost from cache by ip addr
func GetDockerHost(ip string) (*DockerHost, error) {
	if hoststackTmp, ok := DockerHostCache.Load(ip); ok {
		if dh, ok := hoststackTmp.(*DockerHost); ok {
			return dh, nil
		}
	}
	return nil, errors.New("no such host")
}
