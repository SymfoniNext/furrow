package broker

import (
	"bytes"
	"strconv"
	"time"

	"context"

	"github.com/SymfoniNext/furrow/furrow"

	log "github.com/sirupsen/logrus"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/kr/beanstalk"
)

// Fetch jobs from beanstalkd
type beanstalkBroker struct {
	// Connection to beanstalkd.
	conn *beanstalk.Conn
	// Tube name for receiving jobs
	tube *beanstalk.TubeSet
	// Length of time to attempt job reservation
	reserveTime time.Duration

	deadlineHandler  func()
	timeoutHandler   func()
	unmarshalHandler func(uint64)
}

// A seperate connection to Beanstalk is opened for each broker instance.
// This is due to how the beanstalk protocol works.  A job must be
// deleted before a new command can be issued on the same connection.
// https://github.com/kr/beanstalk/issues/6
func New(addr string, queue string) (furrow.Broker, error) {
	conn, err := beanstalk.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	b := beanstalkBroker{
		conn:        conn,
		tube:        beanstalk.NewTubeSet(conn, queue),
		reserveTime: time.Hour * 24 * 7,
	}

	b.timeoutHandler = func() {
	}
	b.deadlineHandler = func() {
		// Timeout is handled via context.WithTimeout
	}
	b.unmarshalHandler = func(id uint64) {
		b.bury(id)
	}

	return b, nil
}

// Bury job
func (b beanstalkBroker) bury(id uint64) {
	if err := b.conn.Bury(id, 0); err != nil {
		log.WithField("id", id).Warnf("Error burying job: %s", err)
	}
}

// Delete job
func (b beanstalkBroker) delete(id uint64) {
	if err := b.conn.Delete(id); err != nil {
		log.WithField("id", id).Warnf("Error deleting job: %s", err)
	}
}

func (b beanstalkBroker) Close() {
	b.conn.Close()
	log.Debug("Closed connection to beanstalkd")
}

func (b beanstalkBroker) GetJob(ctx context.Context) (context.Context, *furrow.Job) {
	id, rawJob, err := b.tube.Reserve(b.reserveTime)
	logFields := log.Fields{
		"jobID": id,
	}

	ctx = context.WithValue(ctx, "ID", id)
	if cerr, ok := err.(beanstalk.ConnError); ok {
		logFields["err"] = cerr.Error()
		log.WithFields(logFields).Warn("Unable to fetch job")
		switch cerr.Err {
		case beanstalk.ErrDeadline:
			// read next ...
			log.WithFields(logFields).Debug("Calling deadline handler")
			b.deadlineHandler()
		case beanstalk.ErrTimeout:
			fallthrough
		default:
			log.WithFields(logFields).Debug("Calling timeout handler")
			b.timeoutHandler()
		}
		return ctx, nil
	}

	// Get the stats for the current job.  This is to get the current deadline for
	// when the job must be completed by.  We use this to set a timeout on the context.
	stats, err := b.conn.StatsJob(id)
	// Do something with error?
	if err != nil {
		log.WithFields(logFields).Warn("Error getting stats", err)
		return ctx, nil
	}

	log.WithFields(logFields).Info("Got job")
	job := &furrow.Job{}
	if rawJob[0] == '{' {
		log.WithFields(logFields).Info("Processing message as JSON")
		buf := bytes.NewBuffer(rawJob)
		err = jsonpb.Unmarshal(buf, job)
	} else {
		err = proto.Unmarshal(rawJob, job)
	}
	if err != nil {
		logFields["error"] = err.Error()
		logFields["job"] = string(rawJob)
		log.WithFields(logFields).Warnf("Error unmarshalling job\n")
		b.unmarshalHandler(id)
		return ctx, nil
	}

	tl, _ := strconv.ParseInt(stats["time-left"], 10, 64)
	duration := time.Second * time.Duration(tl)
	log.WithFields(logFields).Infof("Setting deadline to %d\n", duration)
	ctx, cancel := context.WithTimeout(ctx, duration)
	ctx = context.WithValue(ctx, "cancelFunc", cancel)

	return ctx, job
}

// Finalize the job, cleanup etc.
func (b beanstalkBroker) Finish(ctx context.Context, status furrow.JobStatus) error {
	logFields := log.Fields{
		"jobID": status.ID,
	}

	// Because the docs say so
	if f, ok := CancelFunc(ctx); ok {
		log.WithFields(logFields).Debug("Running context cancel()")
		f()
	}

	// Bury or delete job?  Runner sets jobstatus
	if status.Bury == true {
		log.WithFields(logFields).Info("Burying job")
		b.bury(status.ID)
	} else {
		// Job has run ok, delete job from server
		log.WithFields(logFields).Info("Deleting job")
		b.delete(status.ID)
	}

	if status.Notify == "" {
		log.Debug("Notification not requested")
		return nil // error?
	}

	notification := &furrow.Notification{
		ID:   status.ID,
		Data: status.Output,
	}
	data, err := proto.Marshal(notification)
	if err != nil {
		return err
	}
	tube := &beanstalk.Tube{b.conn, status.Notify}
	_, err = tube.Put(data, 1, 0, time.Second*5)
	if err != nil {
		return err
	}

	return nil
}

func JobID(ctx context.Context) (uint64, bool) {
	id, ok := ctx.Value("ID").(uint64)
	return id, ok
}

func CancelFunc(ctx context.Context) (context.CancelFunc, bool) {
	f, ok := ctx.Value("cancelFunc").(context.CancelFunc)
	return f, ok
}

func BuryJobFunc(ctx context.Context) (func(), bool) {
	f, ok := ctx.Value("buryJobFunc").(func())
	return f, ok
}
