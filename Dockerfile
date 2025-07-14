FROM golang:1.24-bullseye AS buildgo
RUN go version

WORKDIR /branch-out

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .

RUN mkdir -p /tmp/branch-out
RUN CGO_ENABLED=0 go build \
  -ldflags="-X 'github.com/branch-out/branch-out/cmd.builtBy=docker'" \
  -o /tmp/branch-out/branch-out ./main.go

FROM golang:1.24-bullseye

HEALTHCHECK --interval=5m --timeout=3s \
  CMD curl -f http://localhost/health || exit 1

RUN apt-get update && apt-get install -y git curl && rm -rf /var/lib/apt/lists/*

COPY --from=buildgo /tmp/branch-out/ /usr/local/bin/

EXPOSE 8181

ENTRYPOINT ["/usr/local/bin/branch-out"]
