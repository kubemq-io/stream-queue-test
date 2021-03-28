FROM kubemq/gobuilder as builder
ENV ADDR=0.0.0.0
ADD . $GOPATH/github.com/kubemq-io/stream-queue
WORKDIR $GOPATH/github.com/kubemq-io/stream-queue
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -mod=vendor -installsuffix cgo -ldflags="-w -s -X main.version=$VERSION" -o stream-queue-run .
FROM registry.access.redhat.com/ubi8/ubi-minimal

ENV GOPATH=/go
ENV PATH=$GOPATH/bin:$PATH
RUN mkdir /stream-queue
COPY --from=builder $GOPATH/github.com/kubemq-io/stream-queue/stream-queue-run ./stream-queue
RUN chown -R 1001:root  /stream-queue && chmod g+rwX  /stream-queue
WORKDIR stream-queue
USER 1001
CMD ["./stream-queue-run"]

