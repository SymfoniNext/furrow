package jobs

import (
	"bytes"
	"errors"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/SymfoniNext/furrow/broker"
	"github.com/SymfoniNext/furrow/furrow"

	"context"

	docker "github.com/fsouza/go-dockerclient"
)

var (
	// Directory names to use for the in and out mounts in the container
	volumeInMount  = "/in"
	volumeOutMount = "/out"

	ErrEmptyJob = errors.New("Empty job")
)

// Runner for executing jobs via Docker & containers
type jobRunner struct {
	// Docker client
	client *docker.Client

	// Container removal is queued for all clients via this channel
	containerRemoval chan string

	// Docker hub username
	username string
	// Docker hub password
	password string
}

func NewRunner(docker *docker.Client, username string, password string) furrow.Runner {
	job := jobRunner{
		client:           docker,
		containerRemoval: make(chan string),
		username:         username,
		password:         password,
	}

	return job
}

func (j jobRunner) Run(ctx context.Context, job *furrow.Job) furrow.JobStatus {
	if job.GetImage() == "" {
		log.Warnf("Received empty job: (%#v)", job)
		return furrow.JobStatus{
			Err: ErrEmptyJob,
			// No point burying an empty job.
			// But perhaps we need more verbose logging?
			Bury: false,
		}
	}

	jobID, _ := broker.JobID(ctx)
	logFields := log.Fields{
		"requestID": job.GetRequestID(),
		"jobID":     jobID,
		"image":     job.GetImage(),
		"cmd":       job.GetCmd(),
	}

	jobStatus := furrow.JobStatus{
		ID:     jobID,
		Notify: job.GetNotify(),
	}

	// Image doesn't exist, so we need to get it
	// how are we pulling private repos?
	if _, err := j.client.InspectImage(job.GetImage()); err != nil {
		log.WithFields(logFields).Info("Pulling image")
		image := strings.Split(job.GetImage(), ":")
		tag := "latest"
		if len(image) == 2 {
			tag = image[1]
		}
		opts := docker.PullImageOptions{
			Repository: image[0],
			Tag:        tag,
			//OutputStream: // attach to logger
			Context: ctx,
		}
		auth := docker.AuthConfiguration{
			Username: j.username,
			Password: j.password,
		}
		if err := j.client.PullImage(opts, auth); err != nil {
			log.WithFields(logFields).Warn(err)
			jobStatus.Err = err
			jobStatus.Bury = true
			return jobStatus
		}
	}

	var hostConfig *docker.HostConfig
	if job.GetVolumes() != nil {
		logFields["volumes"] = job.GetVolumes()
		binds := make([]string, 0)
		if job.Volumes.GetIn() != "" {
			binds = append(binds, job.Volumes.GetIn()+":"+volumeInMount)
		}
		if job.Volumes.GetOut() != "" {
			binds = append(binds, job.Volumes.GetIn()+":"+volumeOutMount)
		}
		if len(binds) > 0 {
			hostConfig = &docker.HostConfig{
				Binds: binds,
			}
		}
	}

	// option to schedule a service instead?
	log.WithFields(logFields).Info("Creating container")
	container, err := j.client.CreateContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			Image: job.GetImage(),
			Cmd:   job.GetCmd(),
			Env:   job.GetEnv(),
		},
		HostConfig: hostConfig,
	})
	if err != nil {
		log.WithFields(logFields).Info(err)
		jobStatus.Err = err
		jobStatus.Bury = true
		return jobStatus
	}

	logFields["container_id"] = container.ID

	log.WithFields(logFields).Info("Starting container")
	if err := j.client.StartContainerWithContext(container.ID, hostConfig, ctx); err != nil {
		log.WithFields(logFields).Info(err.Error())
		jobStatus.Err = err
		jobStatus.Bury = true
		return jobStatus
	}

	workDone := make(chan struct{}, 1)
	// Want to pick up context cancellations
	error := make(chan error, 1)
	// But still extract any possibly relevant info for the caller
	go func() {
		log.WithFields(logFields).Info("Waiting for container to run")
		// exit code handler - optional code to clean up if failing
		exitCode, err := j.client.WaitContainerWithContext(container.ID, ctx)
		if err != nil {
			error <- err
			return
		}
		if exitCode != 0 {
			jobStatus.ExitCode = exitCode
			jobStatus.Bury = true
			error <- errors.New("None zero exit")
			return
		}

		// if notify (handled by broker)

		log.WithFields(logFields).Info("Attaching to container")
		var buffer bytes.Buffer
		if err := j.client.AttachToContainer(docker.AttachToContainerOptions{
			Container:    container.ID,
			OutputStream: &buffer,
			Logs:         true,
			Stdout:       true,
			Stderr:       true,
		}); err != nil {
			error <- err
			return
		}

		jobStatus.Output = buffer.String()

		log.WithFields(logFields).Info("Removing container")
		j.containerRemoval <- container.ID

		workDone <- struct{}{}
	}()

	select {
	case <-workDone:
		// Wait until container is done
	case err := <-error:
		log.WithFields(logFields).Warn(err)
		jobStatus.Bury = true
		jobStatus.Err = err
	case <-ctx.Done():
		// Remove container?
		log.WithFields(logFields).Warn("Job cancelled after request.  Stopping container.")
		if err := j.client.StopContainer(container.ID, 0); err != nil {
			log.WithFields(logFields).Warn(err)
		}
		jobStatus.Bury = true
		jobStatus.Err = ctx.Err()
	}

	return jobStatus
}

func (j jobRunner) Start() {
	var wg sync.WaitGroup

	wg.Add(1)
	// Container Removal
	// is handled one at a time to prevent stampeding and possible file system errors.
	// (a problem with high-speed jobs and lots of workers)
	go func() {
		defer wg.Done()
		for id := range j.containerRemoval {
			if err := j.client.RemoveContainer(docker.RemoveContainerOptions{
				ID:    id,
				Force: true,
			}); err != nil {
				log.WithField("id", id).WithField("error", err).Warn("Unable to delete container")
			}
		}
	}()

	wg.Wait()
}
