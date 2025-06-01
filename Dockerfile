FROM golang:alpine AS build
WORKDIR /build/snid
COPY go.mod go.sum .
RUN go mod download
COPY *.go .
RUN go build -trimpath -o /usr/bin/snid .

FROM registry.spritsail.io/spritsail/alpine:3.22
COPY --from=build /usr/bin/snid /usr/bin/snid
ENTRYPOINT ["/usr/bin/snid"]
