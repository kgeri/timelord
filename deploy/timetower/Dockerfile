FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/timetower ./cmd/timetower

FROM alpine:3.20

RUN mkdir -p /var/lib/timetower

COPY --from=build /out/timetower /usr/local/bin/timetower

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/timetower"]
CMD ["-listen", "0.0.0.0:8080", "-config", "/var/lib/timetower/config.json", "-state", "/var/lib/timetower/usage.csv"]
