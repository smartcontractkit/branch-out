FROM golang:1.24-bullseye AS buildgo
RUN go version

WORKDIR /branch-out

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .

RUN mkdir -p /tmp/branch-out
RUN CGO_ENABLED=0 go build -o /tmp/branch-out/branch-out ./main.go

FROM alpine:3.22

HEALTHCHECK --interval=5m --timeout=3s \
  CMD curl -f http://localhost/health || exit 1

RUN apk add --no-cache ca-certificates

COPY --from=buildgo /tmp/branch-out/ /usr/local/bin/

EXPOSE 8181

ENTRYPOINT ["/usr/local/bin/branch-out"]
