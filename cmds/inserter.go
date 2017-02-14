package main

import (
	"log"
	"strings"
	"time"

	"github.com/SymfoniNext/furrow/furrow"

	"github.com/namsral/flag"

	"github.com/golang/protobuf/proto"
	"github.com/kr/beanstalk"
)

var (
	host     string
	jobsTube string

	number int

	image      string
	cmd        string
	notifyTube string
)

func init() {
	//
	flag.StringVar(&host, "host", "beanstalk:11300", "Host/port to beanstalk")
	flag.StringVar(&jobsTube, "jobs-tube", "jobs", "Name of jobs tube")
	flag.IntVar(&number, "number", 1, "Number of jobs to send")
	flag.StringVar(&image, "image", "alpine:3.4", "Name of image to run")
	flag.StringVar(&cmd, "cmd-args", "expr 4 * 4", "Arguments for the image")
	flag.StringVar(&notifyTube, "notify-tube", "notify", "Tube to send notifications to")
}

func main() {
	flag.Parse()

	conn, err := beanstalk.Dial("tcp", host)
	if err != nil {
		log.Fatal(err)
	}

	tube := beanstalk.Tube{conn, jobsTube}

	job := &furrow.Job{
		Image:  image,
		Cmd:    strings.Split(cmd, " "),
		Notify: notifyTube,
	}

	data, _ := proto.Marshal(job)

	for i := 0; i < number; i++ {
		id, _ := tube.Put(data, 100, 0, time.Second*60)
		log.Printf("Inserted job %d\n", id)
	}
}
