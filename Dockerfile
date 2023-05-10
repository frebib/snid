FROM golang:alpine AS build
WORKDIR /build/snid
COPY go.mod go.sum .
RUN go mod download
COPY *.go .
RUN go build -o /usr/bin/snid .

FROM alpine
COPY --from=build /usr/bin/snid /usr/bin/snid
ENTRYPOINT ["/usr/bin/snid"]
