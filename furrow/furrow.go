package furrow

import (
	"context"
	"fmt"
)

var (
	appName   string = "furrow"
	buildDate string = "today"
	commitID  string = "XYZ"
)

type Broker interface {
	// Close connection to broker
	Close()
	// Get a job from the broker
	GetJob(context.Context) (context.Context, *Job)
	// Finalize the job with the broker
	Finish(context.Context, JobStatus) error
}

type Runner interface {
	// Start the runner (if needed)
	Start()
	//Stop()
	// Run / execute a job
	Run(context.Context, *Job) JobStatus
}

// The status of the job after processing
type JobStatus struct {
	// Job ID (from broker)
	ID uint64
	// Any errors that occured while running the job
	Err error
	// Whether to mark the job for reprocessing with the broker
	Bury bool
	// Exit code the job
	ExitCode int
	// Whether to send a notification back to the caller via the broker.
	// This is the name of the channel the caller is waiting on.
	Notify string
	// The output from the job
	Output string
}

// Print the application's build info
func Version() {
	fmt.Printf("%s (%s) built %s \n", appName, commitID, buildDate)
}
