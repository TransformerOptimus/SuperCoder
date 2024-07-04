# AI-Developer

## Description
This project is a Go server utilizing the Gin framework, GORM ORM library, and Asynq for task management.
Along with Next.js based GUI

## Prerequisites
Before you proceed, ensure that you have the following installed on your system:
- Docker and Docker Compose
- `direnv`

### Installing Direnv
To handle environment variables more efficiently, install `direnv`:
```bash
# For macOS
brew install direnv

# For Ubuntu
sudo apt-get install direnv
```

After installation, hook direnv into your shell:

```bash 
echo 'eval "$(direnv hook bash)"' >> ~/.bashrc
source ~/.bashrc
```

If you are using a shell other than bash, replace bash with your specific shell (e.g., zsh or fish).

*Note: direnv is one of the suggested ways other ways to setup environment variables are also possible*
## Setup

### 1.Environment Configuration
First, create a .envrc file in the root directory of your project and populate it with the necessary environment variables:
```bash

export AI_DEVELOPER_GITNESS_URL=http://gitness:3000
export AI_DEVELOPER_GITNESS_HOST=gitness:3000

export AI_DEVELOPER_GITNESS_PASSWORD=admin
export AI_DEVELOPER_GITNESS_USER=admin

export AI_DEVELOPER_APP_URL=http://localhost:3000

export AI_DEVELOPER_WORKSPACE_WORKING_DIR=/workspaces
export AI_DEVELOPER_WORKSPACE_SERVICE_ENDPOINT=http://ws:8080

export NEW_RELIC_ENABLED=false
```

To allow direnv to load these settings, run:

```bash
direnv allow .
```
### 2. Build and Run the Go Server, Asynq worker, and Postgres

To build and run the Go server, Asynq worker, and Postgres, execute the following command:

```bash
docker-compose up --build
```

You can now access the UI at http://localhost:3000.