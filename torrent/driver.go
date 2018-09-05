package torrent

import (
	"errors"
	"fmt"
	"os"
	"path"
)

type Distributor struct {
	dir       string
	ctorrent  string
	address   string
	port      int
	quitChan  chan bool
	watchers  map[string]*Watcher
	tracker   *Tracker
	verbosity Verbosity
}

func NewDistributor(
	dir string,
	ctorrentPath string,
	address string,
	port int,
	verbosity Verbosity) (*Distributor, error) {
	SetLoggingVerbosity(verbosity)
	info, err := os.Stat(dir)
	if err != nil {
		LogError("serve path does not exist: %s", err)
		return nil, err
	}
	if !info.IsDir() {
		LogError("serve path is not a directory")
		return nil, errors.New("serve path is not a directory")
	}
	if _, err = os.Stat(ctorrentPath); err != nil {
		LogError("ctorrent binary not found at: %s", ctorrentPath)
		return nil, errors.New(fmt.Sprintf("ctorrent binary not found at: %s", ctorrentPath))
	}
	if port < 1 || port > 65535 {
		LogError("port must be in range 1..65535")
		return nil, errors.New("port must be in range 1..65535")
	}
	return &Distributor{
		dir:       dir,
		ctorrent:  ctorrentPath,
		address:   address,
		port:      port,
		quitChan:  make(chan bool),
		verbosity: verbosity,
	}, nil

}

func (dist *Distributor) Run() {
	dist.Start()
	dist.Wait()
}

func (dist *Distributor) Start() {
	// The basic flow is that we set up a tracker, which listens on a port for HTTP requests. The
	// tracker coordinates peers and torrent files. To each tracker we can attach a set of watchers,
	// which handle monitoring of files.
	SetLoggingVerbosity(dist.verbosity)
	dist.watchers = map[string]*Watcher{
		path.Base(dist.dir): StartWatcher(dist.dir),
	}
	dist.tracker = StartTracker(dist.address, dist.port, dist.ctorrent, dist.watchers)
	LogInfo("distributing %s on %s:%d", dist.dir, dist.address, dist.port)
}

func (dist *Distributor) Wait() {
	<-dist.quitChan
}

func (dist *Distributor) Close() {
	for _, w := range dist.watchers {
		w.Close()
	}
	dist.quitChan <- true
}
