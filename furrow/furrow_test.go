package furrow

import (
	"testing"

	"github.com/golang/protobuf/proto"
)

// Code generation tools run?
func TestJobStruct(t *testing.T) {
	job := &Job{
		Image: "testing",
	}

	data, _ := proto.Marshal(job)
	newJob := &Job{}
	proto.Unmarshal(data, newJob)

	if job.GetImage() != newJob.GetImage() {
		t.Errorf("No job (%s)", job.String())
	}
}

func TestNotificationStruct(t *testing.T) {
	notify := &Notification{
		ID: 1,
	}

	data, _ := proto.Marshal(notify)
	newNotify := &Notification{}
	proto.Unmarshal(data, newNotify)

	if notify.GetID() != newNotify.GetID() {
		t.Errorf("No notification (%s)", notify.String())
	}
}

func ExampleVersion() {
	Version()

	// Output:
	// furrow (XYZ) built today
}
