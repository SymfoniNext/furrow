package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/SymfoniNext/furrow/broker"
	"github.com/SymfoniNext/furrow/furrow"
	"github.com/SymfoniNext/furrow/jobs"

	log "github.com/sirupsen/logrus"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/namsral/flag"
	metrics "github.com/rcrowley/go-metrics"
	"github.com/rcrowley/go-metrics/exp"
)

var (
	dockerEndpoint string
	dockerUsername string
	dockerPassword string

	beanstalkHost string
	jobsTube      string

	workers        int
	publishMetrics string
)

// Job is for passing jobs from reader to worker
type Job struct {
	ctx context.Context
	job *furrow.Job
}

func init() {
	flag.StringVar(&dockerEndpoint, "docker-host", "unix:////var/run/docker.sock", "Address to Docker host")
	flag.StringVar(&beanstalkHost, "beanstalk-host", "beanstalk:11300", "Address and port to beanstalkd")
	flag.StringVar(&jobsTube, "tube", "jobs", "Name of tube to read jobs from")
	flag.IntVar(&workers, "workers", 1, "Number of jobs to process at once")
	flag.StringVar(&dockerUsername, "docker-username", "", "Username for access to Docker hub")
	flag.StringVar(&dockerPassword, "docker-password", "", "Password for username")
	flag.StringVar(&publishMetrics, "publish-metrics", "", "Bind address/port to publish metrics on")
}

func main() {
	flag.Parse()

	furrow.Version()

	if dockerEndpoint == "" || beanstalkHost == "" || jobsTube == "" {
		help()
	}

	log.WithField("DOCKER_HOST", dockerEndpoint).Info("Connecting to Docker")
	client, _ := docker.NewClient(dockerEndpoint)

	stop := make(chan struct{}, 1)
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		sig := <-signals

		log.Infof("Got signal %v, shutting down workers and cancelling running jobs.\n", sig)

		close(stop)

		<-time.After(time.Second * 5)
		log.Info("Force exiting ...")
		os.Exit(1)
	}()

	runner := jobs.NewRunner(client, dockerUsername, dockerPassword)
	go runner.Start()
	//	defer runner.Stop()

	jobTimer := metrics.GetOrRegisterTimer("job.execute_time", metrics.DefaultRegistry)
	jobCounter := metrics.GetOrRegisterCounter("job.executed", metrics.DefaultRegistry)
	if publishMetrics != "" {
		log.Infof("Publishing metrics to %s\n", publishMetrics)
		go func() {
			exp.Exp(metrics.DefaultRegistry)
			log.Warn(http.ListenAndServe(publishMetrics, http.DefaultServeMux))
		}()
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			logFields := log.Fields{
				"WORKER":          worker,
				"BEANSTALKD_HOST": beanstalkHost,
				"TUBE":            jobsTube,
			}
			log.WithFields(logFields).Info("Connecting to beanstalkd")
			b, err := broker.New(beanstalkHost, jobsTube)
			if err != nil {
				log.Fatal(err)
			}

			defer func() {
				log.WithFields(logFields).Info("Worker has stopped")
				b.Close()
				wg.Done()
			}()

			getNextJob := make(chan struct{}, 1)
			jobs := make(chan Job, 1)
			go func() {
				var cancel context.CancelFunc
				cancel = func() {}
				for {
					select {
					case <-stop:
						cancel()
						return
					case <-getNextJob:
						go func() {
							job := Job{}
							ctx := context.Background()
							job.ctx, job.job = b.GetJob(ctx)
							// Should only get nil job because of reserve timeouts.
							// It is therefore safe to just wait again for the next.
							if job.job == nil {
								getNextJob <- struct{}{}
								return
							}
							cancel, _ = broker.CancelFunc(job.ctx)
							jobs <- job

							jobCounter.Inc(1)
						}()
					}
				}
			}()

			for {
				getNextJob <- struct{}{}
				select {
				case <-stop:
					return
				case job := <-jobs:
					jobTimer.Time(func() {
						status := runner.Run(job.ctx, job.job)
						b.Finish(job.ctx, status)
					})
				}
			}

		}(i + 1)
	}
	wg.Wait()
}

func help() {
	flag.Usage()
	os.Exit(1)
}
