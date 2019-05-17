package models

import (
	"errors"
)

type OrderInfo struct {
	ContainerID string
	IpAddr      string
	MasterFlag  string
}

type MatchInfo struct {
	Copied   bool
	SrcHost  string
	DestHost string

	SrcContainer  string
	DestContainer string
}

func CheckOrderInfo(srcOrderInfo, destOrderInfo []OrderInfo) error {
	if len(srcOrderInfo) != len(destOrderInfo) {
		return errors.New("container len not match")
	}

	//TODO, check container status, during backup, all container must be stopped
	return nil
}

func Match(srcOrderInfo, destOrderInfo []OrderInfo) ([]MatchInfo, error) {
	srcMasterIndex := GetMasterIndex(srcOrderInfo)
	destMasterIndex := GetMasterIndex(destOrderInfo)

	if srcMasterIndex == -1 || destMasterIndex == -1 {
		return nil, errors.New("master dose not match")
	}
	res := make([]MatchInfo, 0, 5)
	res = append(res, MatchInfo{
		SrcHost:  srcOrderInfo[srcMasterIndex].IpAddr,
		DestHost: destOrderInfo[destMasterIndex].IpAddr,

		SrcContainer:  srcOrderInfo[srcMasterIndex].ContainerID,
		DestContainer: destOrderInfo[destMasterIndex].ContainerID,
	})

	srcOrderInfo = append(srcOrderInfo[:srcMasterIndex], srcOrderInfo[srcMasterIndex+1:]...)
	destOrderInfo = append(destOrderInfo[:destMasterIndex], destOrderInfo[destMasterIndex+1:]...)

	for i := 0; i < len(srcOrderInfo); i++ {
		res = append(res, MatchInfo{
			SrcHost:  srcOrderInfo[i].IpAddr,
			DestHost: destOrderInfo[i].IpAddr,

			SrcContainer:  srcOrderInfo[i].ContainerID,
			DestContainer: destOrderInfo[i].ContainerID,
		})
	}

	return res, nil
}

func GetMasterIndex(order []OrderInfo) int {
	for i, v := range order {
		if v.MasterFlag == "1" {
			return i
		}
	}

	return -1
}
