FROM public.ecr.aws/docker/library/golang:1.22.3-bookworm AS build-base

RUN apt-get update && apt-get install -y jq && apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR $GOPATH/src/packages/ai-developer/

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY *.go .
COPY app app

FROM build-base AS migrations

RUN apt-get update &&  \
    apt-get install -y postgresql-client && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

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

FROM superagidev/supercoder-python-ide:latest AS worker-development

RUN git config --global user.email "supercoder@superagi.com"
RUN git config --global user.name "SuperCoder"

RUN sudo mkdir -p /opt/venv && sudo chown coder:coder /opt/venv

ENV VIRTUAL_ENV=/opt/venv
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$PATH:$VIRTUAL_ENV/bin"
RUN curl -sSL https://install.python-poetry.org | python3 -

COPY --from=production-base /go/worker /go/worker
COPY ./app/prompts /go/prompts

ENTRYPOINT ["bash", "-c", "/go/worker"]

FROM superagidev/supercoder-python-ide:latest AS executor

RUN git config --global user.email "supercoder@superagi.com"
RUN git config --global user.name "SuperCoder"

RUN sudo mkdir -p /opt/venv && sudo chown coder:coder /opt/venv

ENV HOME /home/coder

COPY --from=production-base /go/executor /go/executor
COPY ./app/prompts /go/prompts

ENTRYPOINT ["bash", "-c", "/entrypoint.d/initialise.sh && /go/executor"]

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
