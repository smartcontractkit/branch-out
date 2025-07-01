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

RUN apk add --no-cache ca-certificates

COPY --from=buildgo /tmp/branch-out/ /usr/local/bin/

EXPOSE 8080
# EXPOSE 8443 ?

ENTRYPOINT ["/usr/local/bin/branch-out"]
