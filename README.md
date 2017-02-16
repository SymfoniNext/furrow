[![Build Status](https://travis-ci.org/SymfoniNext/furrow.svg?branch=master)](https://travis-ci.org/SymfoniNext/furrow)

Furrow
======

A job runner / worker queue processor (currently for `beanstalkd`).  Jobs run in Docker containers.

The message schema is defined via Protocol Buffers (`job.proto`) so it is easy for any language to send in work for furrow to process.  A simple RPC workflow is also available if you want to know when your job has been completed (see further down for an explaination).

## Requirements

* beanstalkd
* Docker

# Starting furrow

First ensure `beanstalkd` is running.   Furrow is all about running jobs in Docker containers so it needs access to the Docker daemon.  Furrow itself is also run in a Docker container!

## Flags (and environment variables)

`-beanstalk-host`:  Address:port number of how furrow can connect to beanstalkd.  Defaults to `beanstalk:11300`.

`-docker-host`: Address to the Docker daemon (where furrow will run jobs).  Defaults to `unix:///var/run/docker.sock`.

`-docker-username`:  Username to Docker hub.  This is so furrow can pull private images.

`-docker-password`:  Password for said username.

`-tube`: The name of the tube to read jobs from.  Defaults to `jobs`.

`-workers`: Number workers to run.  That is the number of jobs that can be processed simultaneously.  Each worker opens a connection to beanstalk.  Defaults to `1`.

`-publish-metrics`: Address and port number to bind to and publish metrics on (starts HTTP server).


Running as a container on single Docker instance:

```
$ docker run -d --restart always --name furrow -v /var/run/docker.sock:/var/run/docker.sock symfoni/furrow:latest **flags**
```

Running as a global service (one instance on each node):

```
$ docker service create --name furrow --mode global --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock symfoni/furrow:latest **flags**
```


# Using furrow

Compose a message using proto3 (there is also support for messages in JSON format) and then send it to beanstalkd on the relevant `"jobs"` tube.  Furrow will then pick up the job and execute it on which ever node picked it up.  If anything goes wrong hopefully there is enough information in the log to fix it.

The furrow worker will block until the job has finished executing, or until the job execution deadline passes.  If the latter happens the job will be cancelled and buried.

The full message and notification schema can be found in `job.proto`.
  
## Notifications (RPC workflow)

It is possible to be notified when a job has finished running by specifying a unique tube name in the `Notification` key.  The sending application then subscribes to that tube and waits for a `Notification` to be sent back.  The notification will include the original job ID and the output from the job.

Tube name uniqueness is the responsibility of the sending app.

## Volumes (sharing large files/datasets)

If you need to pass large amounts of data/files to the job, you can specify a directory in each of the `In` and `Out` keys in `Volumes`.  The container will then be mounted with these directories at `/in` and `/out`.  It is down to to the calling application to ensure these directories exist and that they are cleaned up.

## Sending a job example (PHP)

Use `protoc` to generate the message type API code. `gen/` is the output directory.

```
$ ./bin/protoc --proto_path=. --php_out=gen job.proto
```

Use this `composer.json` file:

```
{
  "require": {
    "pda/pheanstalk": "3.1.0",
    "google/protobuf": "3.2.0"
  }
}
```

Send a job: job is the image `company/some-job:latest` with the flags `-r rflag some-other-flag`.  No notification is requested so it is send and forget.

```
<?php
require_once("./vendor/autoload.php");
require_once("./gen/Furrow/Job.php");
require_once("./gen/GPBMetadata/Job.php");

use Pheanstalk\Pheanstalk;
use Google\Protobuf;
use Google\Protobuf\Internal\RepeatedField;
use Furrow\Job;

$jobs_tube = 'jobs';
$beanstalkd_addr = 'beanstalkd:11300';

$job = new Job;
$job->setRequestID($_SERVER['X-REQUEST-ID']);
$job->setImage("company/some-job:latest");

$cmd = new RepeatedField(\Google\Protobuf\Internal\GPBType::STRING);
$cmd[] = "-r";
$cmd[] = "rflag";
$cmd[] = "some-other-flag";
$job->setCmd($cmd);

$pheanstalk = new Pheanstalk($beanstalkd_addr);
$pheanstalk->useTube($jobs_tube)->put($job->encode());
```

## Writing a job

A job in theory is easy to create.  It just needs to be published as a Docker image and write any relevant output to stdout.  Here's an example job we made for posting simple messages to Slack:  https://github.com/SymfoniNext/slack-post

An exit code != 0 will result in the job being marked as a failure and buried.

## Private images

Private image use have only been tested with Docker Hub.  Fill in the `docker-username` and `docker-password` flags with a relevant Docker hub account.

## Metrics

If enabled the following metrics will be collected and made available on the `/debug/metrics` endpoint.  This endpoint will also include the usual goodies that the `expvar` package exposes.

* Jobs executed counter
* Job execution timer


# Further development of furrow

First get `protoc` and the relevant Go plugin:

```
$ wget https://github.com/google/protobuf/releases/download/v3.0.0/protoc-3.0.0-linux-x86_64.zip
$ unzip protoc-3.0.0-linux-x86_64.zip 
$ go get -u github.com/golang/protobuf/protoc-gen-go
```

Generate the message type API code (place in the `furrow/` package of the repository).

```
$ bin/protoc -I=. --go_out=furrow job.proto
```

Try and include some form of test coverage for your code and ensure all existing tests currently pass.

```
$ make test
```

Build a binary

```
$ CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags "-X 'github.com/SymfoniNext/furrow/furrow.buildDate=$(date)' -X github.com/SymfoniNext/furrow/furrow.commitID=$(git rev-parse --short HEAD) -w -extld ld -extldflags -static" -x -o _furrow .
```

or

```
$ make build
```

# Some TODOs


- [ ] Support Docker named volumes
- [ ] Image exit code handler, different actions on different codes?
- [ ] Jobs as services
- [ ] Do something clever with Docker node metrics (stats) and only accept jobs when the node isn't busy

