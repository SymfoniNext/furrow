package broker

import (
	"encoding/json"
	"testing"
	"time"

	"context"

	"github.com/SymfoniNext/furrow/furrow"
	"github.com/golang/protobuf/proto"
	"github.com/kr/beanstalk"
)

var (
	testTube = "broker-testing"
	job      = &furrow.Job{
		Image:  "testing",
		Cmd:    []string{"one", "two"},
		Env:    []string{"X=Y"},
		Notify: "testing-notify",
	}
)

func init() {
	//	log.SetOutput(ioutil.Discard)
}

func TestNew(t *testing.T) {
	b, err := New("beanstalk:11300", "test")
	if err != nil {
		t.Errorf("Failed connecting to beanstalk %s", err)
	}
	b.Close()
}

func pushJob(b furrow.Broker) uint64 {
	bb := b.(beanstalkBroker)

	data, _ := proto.Marshal(job)
	tube := beanstalk.Tube{bb.conn, testTube}
	id, _ := tube.Put(data, 100, 0, time.Second*3)
	return id
}

func deleteJob(b furrow.Broker, id uint64) {
	bb := b.(beanstalkBroker)
	bb.conn.Delete(id)
}

func TestGetJobHandleTimeout(t *testing.T) {
	b, err := New("beanstalk:11300", testTube)
	if err != nil {
		t.Skipped()
	}
	defer b.Close()

	bb := b.(beanstalkBroker)
	bb.reserveTime = time.Millisecond

	done := make(chan struct{}, 1)
	bb.timeoutHandler = func() {
		done <- struct{}{}
	}

	go func() {
		ctx := context.Background()
		bb.GetJob(ctx)
	}()

	select {
	case <-done:
	case <-time.After(time.Millisecond * 15):
		t.Errorf("GetJob reserve job timeout handler not called")
	}
}

func TestJobUnmarshalHandleError(t *testing.T) {
	b, err := New("beanstalk:11300", testTube)
	if err != nil {
		t.Skipped()
	}
	defer b.Close()

	bb := b.(beanstalkBroker)

	tube := &beanstalk.Tube{bb.conn, testTube}
	id, _ := tube.Put([]byte("junk"), 100, 0, time.Second)
	defer deleteJob(b, id)

	done := make(chan struct{}, 1)
	bb.unmarshalHandler = func(u uint64) {
		done <- struct{}{}
	}

	go func() {
		ctx := context.Background()
		bb.GetJob(ctx)
	}()

	select {
	case <-done:
	case <-time.After(time.Millisecond * 30):
		t.Errorf("GetJob reserve job unmarshal error handler not called")
	}
}

func TestGetJob(t *testing.T) {
	b, err := New("beanstalk:11300", testTube)
	if err != nil {
		t.Skipped()
	}
	defer b.Close()

	pushID := pushJob(b)
	defer deleteJob(b, pushID)

	ctx := context.Background()
	ctx, getJob := b.GetJob(ctx)

	jobID, _ := JobID(ctx)
	if jobID != pushID {
		t.Errorf("ID mismatch %d != %d", jobID, pushID)
	}

	if job.GetImage() != getJob.GetImage() {
		t.Errorf("Expecting %s got %s\n", job.GetImage(), getJob.GetImage())
	}
}

func TestGetJobUnmarshalJSON(t *testing.T) {
	b, err := New("beanstalk:11300", testTube)
	if err != nil {
		t.Skipped()
	}
	defer b.Close()

	pushID := func() uint64 {

		bb := b.(beanstalkBroker)

		data, _ := json.Marshal(job)
		tube := beanstalk.Tube{bb.conn, testTube}
		id, _ := tube.Put(data, 100, 0, time.Second*3)
		return id
	}()
	defer deleteJob(b, pushID)

	ctx := context.Background()
	ctx, getJob := b.GetJob(ctx)

	jobID, _ := JobID(ctx)
	if jobID != pushID {
		t.Errorf("ID mismatch %d != %d", jobID, pushID)
	}

	if job.GetImage() != getJob.GetImage() {
		t.Errorf("Expecting %s got %s\n", job.GetImage(), getJob.GetImage())
	}
}

func TestGetJobCancelsAfterTTR(t *testing.T) {
	b, err := New("beanstalk:11300", testTube)
	if err != nil {
		t.Skipped()
	}
	defer b.Close()

	// 3 second TTR on job
	pushID := pushJob(b)
	defer deleteJob(b, pushID)

	ctx := context.Background()
	ctx, _ = b.GetJob(ctx)

	select {
	case <-ctx.Done():
		// cancel got called
	case <-time.After(time.Second * 4):
		t.Error("GetJob wasn't cancelled")
	}
}

func TestJobIsBuried(t *testing.T) {
	t.Skip()
}

func TestNotify(t *testing.T) {
	t.Skip()

	b, err := New("beanstalk:11300", testTube)
	if err != nil {
		t.Skipped()
	}
	defer b.Close()

	output := "hello this is some output"

	jobStatus := furrow.JobStatus{
		Output: output,
	}

	ctx := context.Background()
	b.Finish(ctx, jobStatus)

	bb := b.(beanstalkBroker)

	tube := &beanstalk.Tube{bb.conn, job.Notify}
	_, body, _ := tube.PeekReady()

	msg := &furrow.Notification{}
	proto.Unmarshal(body, msg)
	if msg.GetData() != output {
		t.Errorf("Expecting \"%s\", got \"%s\"\n", output, msg.GetData())
	}
}
