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

// DockerHost represent docker host info
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

// NewDockerHost initial a new docker host
func NewDockerHost(ip string, logger *log.Logger) *DockerHost {
	return &DockerHost{ip: ip, Logger: logger, Done: make(chan struct{})}
}

// Add add new start container
func (dh *DockerHost) Add(cm *ContainerStats) bool {
	dh.Lock()
	defer dh.Unlock()

	if dh.isKnownContainer(cm.ID) == -1 {
		dh.cStats = append(dh.cStats, cm)
		return true
	}
	return false
}

// Remove remove stopped container from cache
func (dh *DockerHost) Remove(id string) bool {
	dh.Lock()
	defer dh.Unlock()

	if i := dh.isKnownContainer(id); i != -1 {
		// set the container metric to invalid for stopping the collector, also remove container metrics stack
		dh.cStats[i].isInvalid = true
		dh.cStats = append(dh.cStats[:i], dh.cStats[i+1:]...)
		return true
	}
	return false
}

// StopCollect stop collecting current host,
func (dh *DockerHost) StopCollect(rmStop bool) {
	dh.Lock()
	defer dh.Unlock()

	if !dh.closed {
		//set all containerStack status to invalid, to stop all collecting
		for _, containerStack := range dh.cStats {
			containerStack.isInvalid = true
		}
		close(dh.Done)
		dh.closed = true
		dh.Logger.Printf("[%s] stop all container collect", dh.ip)
		if !rmStop {
			StoppedDockerHost.Store(dh.ip, 1)
			return
		}
		config.MonitorInfo.DeleteHost(dh.ip)
	}
}

func (dh *DockerHost) isKnownContainer(cid string) int {
	for i, cm := range dh.cStats {
		if cm.ID == cid || cm.ContainerName == cid {
			return i
		}
	}
	return -1
}

func (dh *DockerHost) IsContainerRunning(cid string) bool {
	dh.RLock()
	defer dh.RUnlock()

	return dh.isKnownContainer(cid) != -1
}

// Length return running container that in monitoring cache
func (dh *DockerHost) Length() int {
	dh.RLock()
	defer dh.RUnlock()

	return cap(dh.cStats)
}

// IsValid check current docker host is valid
func (dh *DockerHost) IsValid() bool {
	dh.RLock()
	defer dh.RUnlock()

	return !dh.closed
}

// AllNames return all running container name
func (dh *DockerHost) AllNames() []string {
	dh.RLock()
	defer dh.RUnlock()

	var names []string
	for _, cm := range dh.cStats {
		names = append(names, cm.ContainerName)
	}

	return names
}

// GetIP return current host ip address
func (dh *DockerHost) GetIP() string {
	return dh.ip
}

// GetContainerStats return all running container metric
func (dh *DockerHost) GetContainerStats() []*ContainerStats {
	return dh.cStats
}

// GetAllLastMemory calculate all latest amount mem that running container used
func (dh *DockerHost) GetAllLastMemory() float64 {
	dh.RLock()
	defer dh.RUnlock()

	var totalMem float64
	for _, cm := range dh.cStats {
		totalMem += cm.GetLatestMemory()
	}

	return totalMem
}

// ContainerEvents  watches for container creation and removal (only
// used when calling `docker stats` without arguments).
func (dh *DockerHost) ContainerEvents(ctx context.Context, started chan<- struct{}, c chan events.Message) {
	defer func() {
		close(c)
		if dh.Cli != nil {
			dh.Logger.Printf("[%s] close docker-cli and remove it from DockerCliList and host list", dh.ip)
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
			dh.Logger.Printf("[%s] listen docker event occurred error : %v", dh.ip, err)
			dh.StopCollect(false)
			return
		case <-dh.Done:
			return
		}
	}
}

// CopyFromContainer copy file from container
func (dh *DockerHost) CopyFromContainer(ctx context.Context, srcContainer, srcPath string) (io.ReadCloser, string, error) {
	content, stat, err := dh.Cli.CopyFromContainer(ctx, srcContainer, srcPath)
	if err != nil {
		return nil, "", err
	}
	dh.Logger.Printf("[%s] copy file-%s with size-%d, from container-%s", dh.ip, srcPath, stat.Size, srcContainer)
	return content, stat.Name, nil
}

// CopyToContainer copy file info container
func (dh *DockerHost) CopyToContainer(ctx context.Context, content io.ReadCloser, destContainer, destPath, name string) error {
	dstStat, err := dh.Cli.ContainerStatPath(ctx, destContainer, destPath)
	if err != nil {
		return err
	}

	// Validate the destination path
	if err := ValidateOutputPathFileMode(dstStat.Mode); err != nil {
		return fmt.Errorf("[%s] destination %s:%s must be a directory or a regular file", dh.ip, destContainer, destPath)
	}

	options := types.CopyToContainerOptions{
		AllowOverwriteDirWithFile: false,
	}

	defer func() {
		content.Close()
		content = nil
	}()
	dh.Logger.Printf("[%s] copy file-%s to container-%s:%s", dh.ip, name, destContainer, destPath)
	return dh.Cli.CopyToContainer(ctx, destContainer, destPath, content, options)
}

// ContainerConsole start a inactive container console
func (dh *DockerHost) ContainerConsole(ctx context.Context, websocketConn *websocket.Conn, container, cmd string) error {
	cli := dh.Cli
	// before do ContainerExecCreate, check the container status, in case of  leaking execIDs
	if containerInfo, err := cli.ContainerInspect(ctx, container); err != nil {
		return err
	} else if !containerInfo.State.Running {
		return fmt.Errorf("[%s] %s is not running please check", dh.ip, container)
	}

	rsp, err := cli.ContainerExecCreate(ctx, container, types.ExecConfig{
		AttachStderr: true,
		AttachStdin:  true,
		AttachStdout: true,
		Cmd:          []string{cmd},
		Detach:       false,
		DetachKeys:   "",
		Privileged:   false,
		Tty:          true,
		User:         "",
		WorkingDir:   "",
	})

	if err != nil {
		return err
	}
	execId := rsp.ID
	HijackedResp, err := cli.ContainerExecAttach(ctx, execId, types.ExecStartCheck{Tty: true, Detach: false})
	if err != nil {
		return err
	}
	defer HijackedResp.Close()

	dh.Logger.Printf("[%s] web console connect to %s  with execId-%s", dh.ip, container, execId)
	errHijack := dh.interactiveExec(websocketConn, HijackedResp)
	dh.Logger.Printf("[%s] web console disconnect to %s  with execId-%s", dh.ip, container, execId)

	return errHijack
}

// ResizeTtyTo resize tty to specific height and width
func (dh *DockerHost) ResizeTtyTo(ctx context.Context, id string, height, width uint) error {
	if height == 0 && width == 0 {
		return nil
	}

	options := types.ResizeOptions{
		Height: height,
		Width:  width,
	}

	err := dh.Cli.ContainerExecResize(ctx, id, options)

	if err != nil {
		dh.Logger.Printf("[%s] Error resize: %s\r", dh.ip, err.Error())
	}
	return err
}

// hijackRequest manage the tcp connection
func (dh *DockerHost) interactiveExec(websocketConn *websocket.Conn, HijackedResp types.HijackedResponse) error {
	tcpConn, resultBufReader := HijackedResp.Conn, HijackedResp.Reader
	defer HijackedResp.CloseWrite()
	defer tcpConn.Close()

	errorChan := make(chan error, 1)
	go resultFromDockerToWebsocket(websocketConn, resultBufReader, errorChan)
	go cmdFromWebsocketToDocker(websocketConn, tcpConn, errorChan)

	select {
	case err := <-errorChan:
		tcpConn.Write([]byte("exit\n"))
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
			return err
		}
	case <-dh.Done:
		tcpConn.Write([]byte("exit\n"))
	}

	return nil
}

// cmdFromWebsocketToContainer
func cmdFromWebsocketToDocker(websocketConn *websocket.Conn, tcpConn net.Conn, errorChan chan error) {
	for {
		_, in, err := websocketConn.ReadMessage()
		if err != nil {
			tcpConn.Write([]byte("exit\n"))
			errorChan <- err
			break
		}
		_, err = tcpConn.Write(in)
		if err != nil {
			errorChan <- err
			break
		}
	}
}

// resultFromDockerToWebsocket
func resultFromDockerToWebsocket(websocketConn *websocket.Conn, br *bufio.Reader, errorChan chan error) {
	for {
		out := make([]byte, 1024)
		n, err := br.Read(out)
		if err != nil {
			errorChan <- err
			break
		}

		processedOutput := validString(string(out[:n]))
		err = websocketConn.WriteMessage(websocket.TextMessage, []byte(processedOutput))
		if err != nil {
			errorChan <- err
			break
		}
	}
}

// copy from portainer
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

// AllStoppedDHIP return all stopped docker host ip addr
func AllStoppedDHIP() []string {
	ips := make([]string, 0, config.MonitorInfo.GetHostsLen())
	StoppedDockerHost.Range(func(key, value interface{}) bool {
		ip, _ := key.(string)
		ips = append(ips, ip)
		return true
	})

	return ips
}

// StopAllDockerHost stopped collection all docker host in cache
func StopAllDockerHost() {
	times := 0
	for len(AllStoppedDHIP()) != config.MonitorInfo.GetHostsLen() {
		if times >= 2 {
			break
		}
		DockerHostCache.Range(func(key, value interface{}) bool {
			if dh, ok := value.(*DockerHost); ok && dh.IsValid() {
				dh.StopCollect(true)
			}
			time.Sleep(5 * time.Microsecond)
			return true
		})
		times++
	}
	log.Printf("stop all docker host monitoring with %d loop", times)
}
