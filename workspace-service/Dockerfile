FROM public.ecr.aws/docker/library/golang:1.22.3-bookworm AS build-base

WORKDIR $GOPATH/src/packages/workspace-service/

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY *.go .
COPY app app

FROM build-base AS server-development

ENV PORT 8080
ENV GIN_MODE debug
EXPOSE 8080

ENTRYPOINT ["go", "run", "server.go"]

FROM build-base AS production-base

WORKDIR $GOPATH/src/packages/workspace-service/

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/server server.go

FROM debian:bookworm-slim as development

RUN apt-get update && apt-get install -y rsync sudo git && apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /app/workspace-service

COPY --from=production-base --chown=coder /go/server /app/workspace-service/server
COPY --chown=coder templates /templates

ENTRYPOINT ["/app/workspace-service/server"]

FROM public.ecr.aws/docker/library/debian:bookworm-slim as production

RUN apt-get update && apt-get install -y rsync sudo git && apt-get clean && rm -rf /var/lib/apt/lists/*

RUN groupadd -g 1000 coder
RUN useradd -u 1000 -g coder coder
RUN usermod -aG sudo coder
RUN echo 'coder ALL=NOPASSWD: ALL' >> /etc/sudoers

ENV PORT 8080
EXPOSE 8080
ENV GIN_MODE release

WORKDIR /app/workspace-service
RUN chown -R coder:coder /app

USER coder

COPY --from=production-base --chown=coder /go/server /app/workspace-service/server
COPY --chown=coder templates /templates

ENTRYPOINT ["/app/workspace-service/server"]
