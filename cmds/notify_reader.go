package main

import (
	"log"
	"time"

	"github.com/SymfoniNext/furrow/furrow"

	"github.com/namsral/flag"

	"github.com/golang/protobuf/proto"
	"github.com/kr/beanstalk"
)

var (
	host       string
	notifyTube string
)

func init() {
	flag.StringVar(&host, "host", "beanstalk:11300", "Host/port to beanstalk")
	flag.StringVar(&notifyTube, "notify-tube", "notify", "Tube to read notifications from")

}

func main() {
	//
	flag.Parse()

	conn, err := beanstalk.Dial("tcp", host)
	if err != nil {
		log.Fatal(err)
	}

	tube := beanstalk.NewTubeSet(conn, notifyTube)

	for {
		id, body, err := tube.Reserve(time.Hour * 24 * 7)
		if err != nil {
			log.Println(err)
			continue
		}

		notification := &furrow.Notification{}
		proto.Unmarshal(body, notification)

		log.Printf("Job %d; Result: %s\n", notification.GetID(), notification.GetData())

		conn.Delete(id)
	}
}
