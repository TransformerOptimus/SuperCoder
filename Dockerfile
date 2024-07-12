FROM public.ecr.aws/docker/library/golang:1.22.3-bookworm AS build-base

RUN apt-get update && apt-get install -y jq postgresql-client && apt-get clean && rm -rf /var/lib/apt/lists/*

RUN useradd -m coder

USER coder
WORKDIR /home/coder

RUN git config --global --add safe.directory /workspaces
RUN git config --global user.email "supercoder@superagi.com"
RUN git config --global user.name "SuperCoder"

WORKDIR $GOPATH/src/packages/ai-developer/

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY *.go .
COPY app app

FROM build-base AS migrations

RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

COPY ./bin/migrations.sh /opt/migrations.sh

ENTRYPOINT ["bash", "-c", "/opt/migrations.sh"]


FROM build-base AS server-development

ENV PORT 8080
ENV GIN_MODE debug
EXPOSE 8080

ENTRYPOINT ["go", "run", "server.go"]

FROM build-base AS production-base

WORKDIR $GOPATH/src/packages/ai-developer/

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/server server.go
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/worker worker.go
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/executor executor.go


FROM build-base AS executor-base

WORKDIR $GOPATH/src/packages/ai-developer/

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/executor executor.go


FROM build-base AS worker-development

ENTRYPOINT ["go", "run", "worker.go"]

FROM superagidev/supercoder-python-ide:latest AS python-executor

RUN git config --global user.email "supercoder@superagi.com"
RUN git config --global user.name "SuperCoder"

RUN sudo mkdir -p /opt/venv && sudo chown coder:coder /opt/venv

ENV HOME /home/coder

COPY --from=executor-base /go/executor /go/executor
COPY ./app/prompts /go/prompts

ENTRYPOINT ["bash", "-c", "/entrypoint.d/initialise.sh && /go/executor"]

FROM superagidev/supercoder-node-ide:latest AS node-executor

RUN git config --global user.email "supercoder@superagi.com"
RUN git config --global user.name "SuperCoder"


ENV HOME /home/coder

COPY --from=executor-base /go/executor /go/executor
COPY ./app/prompts /go/prompts

ENTRYPOINT ["bash", "-c", "/go/executor"]

FROM public.ecr.aws/docker/library/debian:bookworm-slim as production

# install git
RUN apt-get update &&  \
    apt-get install -y git zip \
    && apt-get clean

RUN git config --global user.email "supercoder@superagi.com"
RUN git config --global user.name "SuperCoder"

WORKDIR /go

ENV PORT 8080
ENV GIN_MODE release
EXPOSE 8080

COPY --from=production-base /go/server /go/server
COPY --from=production-base /go/worker /go/worker
COPY ./app/prompts /go/prompts
