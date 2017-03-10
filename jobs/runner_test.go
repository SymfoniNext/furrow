package jobs

import (
	"testing"
	"time"

	"context"

	"github.com/SymfoniNext/furrow/furrow"

	docker "github.com/fsouza/go-dockerclient"
)

var (
	volumeIn  = "/tmp/in"
	volumeOut = "/tmp/out"
	endpoint  = "unix:///var/run/docker.sock"
	job       = &furrow.Job{
		Image:  "alpine:3.4",
		Cmd:    []string{"echo", "hi"},
		Env:    []string{"X=123"},
		Notify: "testing-notify",
		Volumes: &furrow.Job_Volume{
			In:  volumeIn,
			Out: volumeOut,
		},
	}
)

func runner() (furrow.Runner, error) {
	// check endpoint exists first
	client, err := docker.NewClient(endpoint)
	if err != nil {
		return nil, err
	}

	return NewRunner(client, "", ""), nil
}

func TestNewRunner(t *testing.T) {
	t.Skip()
}

func TestRunErrorsWhenJobEmpty(t *testing.T) {
	runner, err := runner()
	if err != nil {
		t.Skip()
	}
	go runner.Start()

	ctx := context.Background()
	done := make(chan furrow.JobStatus, 1)
	job := &furrow.Job{}

	go func() {
		status := runner.Run(ctx, job)
		done <- status
	}()

	select {
	case status := <-done:
		if status.Err != ErrEmptyJob {
			t.Errorf("Expecting error '%s', got '%v'", ErrEmptyJob, status.Err)
		}
	case <-time.After(time.Second):
		t.Error("Timed out")
	}
}

func TestRunCanCancel(t *testing.T) {
	runner, err := runner()
	if err != nil {
		t.Skip()
	}
	go runner.Start()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{}, 1)

	go func() {
		defer func() {
			done <- struct{}{}
		}()
		status := runner.Run(ctx, job)
		if status.Err.Error() != "context canceled" {
			t.Errorf("Expecting context canceled error, got %v", err)
		}
	}()

	go func() {
		time.Sleep(time.Millisecond * 35)
		cancel()
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Timed out, expecting context canceled error")
	}
}

func TestRunRunsDockerContainer(t *testing.T) {
	runner, err := runner()
	if err != nil {
		t.Skip()
	}
	go runner.Start()

	ctx := context.Background()
	done := make(chan furrow.JobStatus, 1)

	go func() {
		status := runner.Run(ctx, job)
		if status.Err != nil {
			t.Errorf("Got error: %s", status.Err)
		}
		done <- status
	}()

	select {
	case status := <-done:
		if status.Output != "hi\n" {
			t.Errorf("Expecting output 'hi', got '%s'", status.Output)
		}
	case <-time.After(time.Second * 3):
		t.Error("Timed out")
	}
}

func TestRunSetExitCode(t *testing.T) {
	runner, err := runner()
	if err != nil {
		t.Skip()
	}
	go runner.Start()

	ctx := context.Background()
	done := make(chan furrow.JobStatus, 1)

	go func() {
		job.Cmd = []string{"false"}
		status := runner.Run(ctx, job)
		done <- status
	}()

	select {
	case status := <-done:
		if status.ExitCode != 1 {
			t.Errorf("Expecting 1, got %v", status.ExitCode)
		}
	case <-time.After(time.Second * 3):
		t.Error("Timed out")
	}
}

func TestCancelledJobCallsBury(t *testing.T) {
	t.Skip()

	runner, err := runner()
	if err != nil {
		t.Skip()
	}
	go runner.Start()

	buriedJob := make(chan struct{}, 1)

	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	ctx = context.WithValue(ctx, "buryJobFunc", func() {
		buriedJob <- struct{}{}
	})

	go func() {
		job.Cmd = []string{"sleep", "2"}
		runner.Run(ctx, job)
	}()

	select {
	case <-buriedJob:
		// good
	case <-time.After(time.Second * 2):
		t.Error("Timed out, buryJobFunc from context not called")
	}
}
