#!/bin/bash

function initialise() {
    cd "$1" || exit

    export PATH=$PATH:$1/.venv/bin:/opt/poetry/bin
    echo $PATH

    export HOME=/home/coder

    if [ ! -d ".venv" ]; then
        echo "Creating virtual environment..."
        python3 -m venv .venv
    else
        echo "Virtual environment already exists, skipping creation..."
    fi

    source .venv/bin/activate
    echo "Using python at $(which python3)"

    if ! command -v poetry &> /dev/null; then
        echo "Poetry could not be found, installing poetry..."
        curl -sSL https://install.python-poetry.org | python3 -
    else
        echo "Poetry already installed, skipping installation..."
    fi

    echo "Using poetry at $(which poetry)"

    echo "Updating lock file..."
    poetry lock --no-update

    echo "Installing dependencies with Poetry..."
    poetry install

    mkdir -p .vscode || true

    # Check if .vscode/settings.json exists
    if [ ! -f .vscode/settings.json ]; then
        touch .vscode/settings.json
        echo "{ \"python.defaultInterpreterPath\":\"$(pwd)/.venv/bin/python\", \"terminal.integrated.env.linux\": { \"PYTHONPATH\": \"$(pwd)\", \"PATH\":\"$PATH\" } }" > .vscode/settings.json
    fi
}

if [ -d "/workspaces" ]; then
    # List all directories in /workspaces
    workspaces=$(ls /workspaces)
    for workspace in $workspaces; do
        initialise "/workspaces/$workspace"
    done
fi

if [ -d "/project" ]; then
    initialise "/project"
fi