FROM alpine:3.6
ADD _furrow /bin/furrow
ENTRYPOINT ["/bin/furrow"]
