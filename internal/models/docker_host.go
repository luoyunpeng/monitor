package models

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/gorilla/websocket"
	"github.com/luoyunpeng/monitor/internal/config"
)

// DockerHost
type DockerHost struct {
	sync.RWMutex
	cStats []*ContainerStats
	//indicate which host this stats belong to
	ip     string
	Logger *log.Logger
	Cli    *client.Client
	// Done close means that this host has been canceled for monitoring
	Done   chan struct{}
	closed bool
}

// NewContainerHost
func NewDockerHost(ip string, logger *log.Logger) *DockerHost {
	return &DockerHost{ip: ip, Logger: logger, Done: make(chan struct{})}
}

func (dh *DockerHost) Add(cm *ContainerStats) bool {
	dh.Lock()

	if dh.isKnownContainer(cm.ID) == -1 {
		dh.cStats = append(dh.cStats, cm)
		dh.Unlock()
		return true
	}
	dh.Unlock()
	return false
}

func (dh *DockerHost) Remove(id string) {
	dh.Lock()

	if i := dh.isKnownContainer(id); i != -1 {
		// set the container metric to invalid for stopping the collector, also remove container metrics stack
		dh.cStats[i].isInvalid = true
		dh.cStats = append(dh.cStats[:i], dh.cStats[i+1:]...)
	}
	dh.Unlock()
}

func (dh *DockerHost) StopCollect() {
	dh.Lock()
	if !dh.closed {
		//set all containerStack status to invalid, to stop all collecting
		for _, containerStack := range dh.cStats {
			containerStack.isInvalid = true
		}
		close(dh.Done)
		dh.closed = true
		dh.Logger.Println("stop all container collect")
		StoppedDockerHost.Store(dh.ip, struct{}{})
	}
	dh.Unlock()
}

func (dh *DockerHost) isKnownContainer(cid string) int {
	for i, cm := range dh.cStats {
		if cm.ID == cid || cm.ContainerName == cid {
			return i
		}
	}
	return -1
}

func (dh *DockerHost) Length() int {
	dh.RLock()
	cmsLen := cap(dh.cStats)
	dh.RUnlock()

	return cmsLen
}

func (dh *DockerHost) IsValid() bool {
	dh.RLock()
	valid := !dh.closed
	dh.RUnlock()

	return valid
}

func (dh *DockerHost) AllNames() []string {
	dh.RLock()

	var names []string
	for _, cm := range dh.cStats {
		names = append(names, cm.ContainerName)
	}
	dh.RUnlock()

	return names
}

func (dh *DockerHost) GetIP() string {
	return dh.ip
}

func (dh *DockerHost) GetContainerStats() []*ContainerStats {
	return dh.cStats
}

func (dh *DockerHost) GetAllLastMemory() float64 {
	dh.RLock()

	var totalMem float64
	for _, cm := range dh.cStats {
		totalMem += cm.GetLatestMemory()
	}
	dh.RUnlock()

	return totalMem
}

// monitorContainerEvents watches for container creation and removal (only
// used when calling `docker stats` without arguments).
func (dh *DockerHost) ContainerEvents(ctx context.Context, started chan<- struct{}, c chan events.Message) {
	defer func() {
		close(c)
		if dh.Cli != nil {
			dh.Logger.Println("close docker-cli and remove it from DockerCliList and host list")
			DockerHostCache.Delete(dh.ip)
			dh.Cli.Close()
		}
	}()

	f := filters.NewArgs()
	f.Add("type", "container")
	options := types.EventsOptions{
		Filters: f,
	}

	eventq, errq := dh.Cli.Events(ctx, options)

	// Whether we successfully subscribed to eventq or not, we can now
	// unblock the main goroutine.
	close(started)

	// wait for container events happens
	for {
		select {
		case event := <-eventq:
			c <- event
		case err := <-errq:
			dh.Logger.Printf("host: err happen when listen docker event: %v", err)
			dh.StopCollect()
			return
		case <-dh.Done:
			return
		}
	}
}

func (dh *DockerHost) CopyFromContainer(ctx context.Context, srcContainer, srcPath string) (io.ReadCloser, string, error) {
	content, stat, err := dh.Cli.CopyFromContainer(ctx, srcContainer, srcPath)
	if err != nil {
		return nil, "", err
	}
	dh.Logger.Printf("copy file-%s with size-%d, from container-%s in host-%s", srcPath, stat.Size, srcContainer, dh.ip)
	return content, stat.Name, nil
}

func (dh *DockerHost) CopyToContainer(ctx context.Context, content io.ReadCloser, destContainer, destPath, name string) error {
	dstStat, err := dh.Cli.ContainerStatPath(ctx, destContainer, destPath)
	if err != nil {
		return err
	}

	// Validate the destination path
	if err := ValidateOutputPathFileMode(dstStat.Mode); err != nil {
		return errors.New(fmt.Sprintf("destination %s:%s must be a directory or a regular file", destContainer, destPath))
	}

	options := types.CopyToContainerOptions{
		AllowOverwriteDirWithFile: false,
	}

	defer func() {
		content.Close()
		content = nil
	}()
	dh.Logger.Printf("copy file-%s to container-%s:%s in host-%s", name, destContainer, destPath, dh.ip)
	return dh.Cli.CopyToContainer(ctx, destContainer, destPath, content, options)
}

// ContainerConsole container console
func (dh *DockerHost) ContainerConsole(ctx context.Context, websocketConn *websocket.Conn, container, cmd string) error {
	rsp, err := dh.Cli.ContainerExecCreate(ctx, container, types.ExecConfig{
		AttachStderr: true,
		AttachStdin:  true,
		AttachStdout: true,
		Cmd:          []string{cmd},
		Detach:       false,
		DetachKeys:   "ctrl-p,ctrl-q",
		Privileged:   false,
		Tty:          true,
		User:         "",
		WorkingDir:   "",
	})
	if err != nil {
		return err
	}
	execId := rsp.ID
	HijackedResp, err := dh.Cli.ContainerExecAttach(ctx, execId, types.ExecStartCheck{Tty: true, Detach: false})
	if err != nil {
		return err
	}
	defer HijackedResp.Close()

	erre := hijackRequest(websocketConn, HijackedResp)
	if erre != nil {
		return err
	}

	return nil
}

// hijackRequest manage the tcp connection
func hijackRequest(websocketConn *websocket.Conn, resp types.HijackedResponse) error {
	tcpConn, brw := resp.Conn, resp.Reader
	defer tcpConn.Close()

	errorChan := make(chan error, 1)
	cmdChan := make(chan string, 1)
	go streamFromTCPConnToWebsocketConn(websocketConn, brw, errorChan, cmdChan)
	go streamFromWebsocketConnToTCPConn(websocketConn, tcpConn, errorChan, cmdChan)

	err := <-errorChan
	if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
		return err
	}

	return nil
}

// streamFromWebsocketConnToTCPConn
func streamFromWebsocketConnToTCPConn(websocketConn *websocket.Conn, tcpConn net.Conn, errorChan chan error, cmdChan chan<- string) {
	for {
		_, in, err := websocketConn.ReadMessage()
		if err != nil {
			errorChan <- err
			break
		}
		cmdChan <- string(in[:])
		if !strings.HasSuffix(string(in), "\n") {
			in = append(in, []byte("\n")...)
		}
		_, err = tcpConn.Write(in)
		if err != nil {
			errorChan <- err
			break
		}
	}
}

// streamFromTCPConnToWebsocketConn
func streamFromTCPConnToWebsocketConn(websocketConn *websocket.Conn, br *bufio.Reader, errorChan chan error, cmdChan <-chan string) {
	for {
		out := make([]byte, 1024)
		n, err := br.Read(out)
		if err != nil {
			errorChan <- err
			break
		}

		processedOutput := validString(string(out[:n]))
		cmd := <-cmdChan
		if strings.TrimSpace(cmd) == strings.TrimSpace(processedOutput) {
			continue
		}
		err = websocketConn.WriteMessage(websocket.TextMessage, []byte(processedOutput))
		if err != nil {
			errorChan <- err
			break
		}
	}
}

func validString(s string) string {
	if !utf8.ValidString(s) {
		v := make([]rune, 0, len(s))
		for i, r := range s {
			if r == utf8.RuneError {
				_, size := utf8.DecodeRuneInString(s[i:])
				if size == 1 {
					continue
				}
			}
			v = append(v, r)
		}
		s = string(v)
	}
	return s
}

// ValidateOutputPathFileMode validates the output paths of the `cp` command and serves as a
// helper to `ValidateOutputPath`
func ValidateOutputPathFileMode(fileMode os.FileMode) error {
	switch {
	case fileMode&os.ModeDevice != 0:
		return errors.New("got a device")
	case fileMode&os.ModeIrregular != 0:
		return errors.New("got an irregular file")
	}
	return nil
}

// GetHostContainerInfo return Host's container info
func GetHostContainerInfo(ip string) []string {
	if dh, err := GetDockerHost(ip); err == nil {
		return dh.AllNames()
	}

	return nil
}

func AllStoppedDHIP() []string {
	ips := make([]string, 0, len(config.MonitorInfo.Hosts))
	StoppedDockerHost.Range(func(key, value interface{}) bool {
		ip, _ := key.(string)
		ips = append(ips, ip)
		return true
	})

	return ips
}

func StopAllDockerHost() {
	times := 0
	for len(AllStoppedDHIP()) != len(config.MonitorInfo.Hosts) {
		if times >= 2 {
			break
		}
		DockerHostCache.Range(func(key, value interface{}) bool {
			if dh, ok := value.(*DockerHost); ok && dh.IsValid() {
				dh.StopCollect()
			}
			time.Sleep(5 * time.Microsecond)
			return true
		})
		times++
	}
	log.Printf("stop all docker host monitoring with %d loop", times)
}
