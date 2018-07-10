#!/bin/bash
echo "IP value is not given, default localhsot, please consider your local test env"
cd $GOPATH/src/github.com/luoyunpeng/monitor
go build -tags=jsonniter monitor.go
