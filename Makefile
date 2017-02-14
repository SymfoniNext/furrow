TARGET = _furrow
IMAGE = symfoni/furrow
GITCOMMIT = `git rev-parse --short HEAD`
DATE = `date`

.PHONEY: clean test

clean:
	rm $(TARGET) 
	go clean

test:
	go vet
	golint
	go test -covermode=count ./broker
	go test -covermode=count ./jobs
	go test -covermode=count ./furrow


build: test
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags "-X 'github.com/SymfoniNext/furrow/furrow.buildDate=$(DATE)' -X github.com/SymfoniNext/furrow/furrow.commitID=$(GITCOMMIT) -w -extld ld -extldflags -static" -x -o $(TARGET) .

docker: build
	docker build -t $(IMAGE):$(GITCOMMIT) .
