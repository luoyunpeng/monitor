#!/bin/bash
echo "IP value is not given, default localhsot, please consider your local test env, and gin is in debug mode"
cd $GOPATH/src/github.com/luoyunpeng/monitor
go build -tags=jsonniter monitor.go
