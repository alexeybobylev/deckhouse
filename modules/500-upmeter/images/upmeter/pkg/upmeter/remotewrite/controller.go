package remotewrite

import (
	"context"
	"fmt"
	"time"

	"github.com/flant/shell-operator/pkg/kube"
	log "github.com/sirupsen/logrus"

	"upmeter/pkg/check"
	"upmeter/pkg/crd"
	v1 "upmeter/pkg/crd/v1"
	dbcontext "upmeter/pkg/upmeter/db/context"
)

type Exporter interface {
	Export(origin string, episodes []*check.DowntimeEpisode, slotSize int64) error
}

// ControllerConfig configures and creates a Controller
type ControllerConfig struct {
	// collect/export period should be less than episodes update period to catch up with data after downtimes
	Period time.Duration

	// monitoring config objects in kubernetes
	Kubernetes kube.KubernetesClient

	// read metrics and track exporter state in the DB
	DbCtx        *dbcontext.DbContext
	OriginsCount int

	Logger *log.Logger
}

func (cc *ControllerConfig) Controller() *Controller {
	var (
		kubeMonLogger    = cc.Logger.WithField("who", "kubeMonitor")
		syncLogger       = cc.Logger.WithField("who", "syncers")
		controllerLogger = cc.Logger.WithField("who", "controller")
	)

	kubeMonitor := crd.NewRemoteWriteMonitor(cc.Kubernetes, kubeMonLogger)
	storage := newStorage(cc.DbCtx, cc.OriginsCount)
	syncers := newSyncers(storage, cc.Period, syncLogger)

	controller := &Controller{
		kubeMonitor: kubeMonitor,
		syncers:     syncers,
		logger:      controllerLogger,
	}

	return controller
}

// Controller links metrics syncers with configs from CR monitor
type Controller struct {
	kubeMonitor *crd.RemoteWriteMonitor
	syncers     *syncers
	logger      *log.Entry
}

func (c *Controller) Start(ctx context.Context) error {
	// Monitor tracks the exporter configuration in kubernetes. It is important to subscribe (add event callback)
	// before monitor starts because informers are created during monitor.Start(ctx) call.
	c.logger.Debugln("subscribing to k8s events")
	c.kubeMonitor.Subscribe(&updateHandler{
		syncers: c.syncers,
		logger:  c.logger.WithField("who", "updateHandler"),
	})
	c.logger.Debugln("starting k8s monitor")
	err := c.kubeMonitor.Start(ctx)
	if err != nil {
		return fmt.Errorf("cannot start monitor: %v", err)
	}

	// ID syncers runs and stops metrics exporters. Here we read configs and add them one by one.
	c.logger.Debugln("getting k8s CRs list")
	rws, err := c.kubeMonitor.List()
	if err != nil {
		return fmt.Errorf("cannot get initial list of upmeterremotewrite objects: %v", err)
	}
	c.logger.Debugf("found %d k8s CRs", len(rws))
	for _, rw := range rws {
		c.logger.Debugf("adding %q syncer", rw.Name)
		err = c.syncers.Add(ctx, newExportConfig(rw))
		if err != nil {
			c.kubeMonitor.Stop()
			c.syncers.stop()
			return fmt.Errorf("cannot add remote_write syncer %q: %v", rw.Name, err)
		}
	}
	c.logger.Debugln("starting syncers")
	err = c.syncers.start(ctx)
	if err != nil {
		return fmt.Errorf("cannot start syncers: %v", err)
	}

	return nil
}

func (c *Controller) Export(origin string, episodes []*check.DowntimeEpisode, slotSize int64) error {
	return c.syncers.AddEpisodes(origin, episodes, slotSize)
}

func (c *Controller) Stop() {
	c.kubeMonitor.Stop()
	c.syncers.stop()

}

// updateHandler implements the interface required to subscribe to object changes in CR monitor
type updateHandler struct {
	syncers *syncers
	logger  *log.Entry
}

func (s *updateHandler) OnAdd(rw *v1.RemoteWrite) {
	err := s.syncers.Add(context.Background(), newExportConfig(rw))

	if err != nil {
		s.logger.Errorf("cannot add remote_write exporter %q: %v", rw.Name, err)
	}
}

func (s *updateHandler) OnModify(rw *v1.RemoteWrite) {
	err := s.syncers.Add(context.Background(), newExportConfig(rw))

	if err != nil {
		s.logger.Errorf("cannot update remote_write exporter %q: %v", rw.Name, err)
	}
}

func (s *updateHandler) OnDelete(rw *v1.RemoteWrite) {
	config := newExportConfig(rw)
	s.syncers.Delete(config) // TODO: ctx? final exporter requests can take some time
}
