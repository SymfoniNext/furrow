FROM alpine:3.4
ADD _furrow /bin/furrow
ENTRYPOINT ["/bin/furrow"]
