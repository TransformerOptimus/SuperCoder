FROM codercom/code-server:4.89.1-debian

RUN sudo apt-get update \
    && sudo apt-get upgrade -y \
    && sudo apt-get install -y make build-essential libssl-dev \
    && sudo apt-get install -y libbz2-dev libreadline-dev libsqlite3-dev wget curl llvm \
    && sudo apt-get install -y libncurses5-dev  libncursesw5-dev xz-utils tk-dev \
    && sudo apt-get clean \
    && sudo apt-get auto-remove -y \
    && sudo rm -rf /var/cache/apt/* \
    && sudo rm -rf /var/lib/apt/lists/* \
    && sudo rm -rf /tmp/*

ARG PYTHON_VERSION=3.12.3

WORKDIR /tmp/python
RUN wget https://www.python.org/ftp/python/$PYTHON_VERSION/Python-$PYTHON_VERSION.tgz \
    && tar xvf Python-$PYTHON_VERSION.tgz \
    && cd Python-$PYTHON_VERSION \
    && ./configure \
    && make -j8 \
    && sudo make install \
    && cd .. \
    && sudo rm -rf Python-$PYTHON_VERSION \
    && sudo rm Python-$PYTHON_VERSION.tgz

RUN sudo mkdir -p /opt/poetry && sudo chown coder:coder /opt/poetry

RUN sudo apt-get update \
    && sudo apt-get install -y jq \
    && sudo apt-get clean \
    && sudo apt-get auto-remove -y \
    && sudo rm -rf /var/cache/apt/* \
    && sudo rm -rf /var/lib/apt/lists/* \
    && sudo rm -rf /tmp/*

WORKDIR /home/coder

ENV PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=on \
    PIP_DEFAULT_TIMEOUT=100 \
    POETRY_VERSION=1.8.3 \
    POETRY_HOME="/opt/poetry" \
    POETRY_NO_INTERACTION=1 \
    POETRY_VIRTUALENVS_CREATE=false

ENV PATH="$PATH:$POETRY_HOME/bin"

RUN wget https://open-vsx.org/api/ms-python/python/2024.4.1/file/ms-python.python-2024.4.1.vsix && \
    code-server --install-extension ms-python.python-2024.4.1.vsix && \
    rm ms-python.python-2024.4.1.vsix

RUN git config --global --add safe.directory /workspaces
RUN git config --global user.email "supercoder@superagi.com"
RUN git config --global user.name "SuperCoder"

RUN mkdir -p /home/coder/.config/code-server/
COPY --chown=coder config.yaml /home/coder/.config/code-server/config.yaml
COPY --chown=coder settings.json /home/coder/.local/share/code-server/User/settings.json

RUN sudo mkdir /entrypoint.d && sudo chown -R coder:coder /entrypoint.d
COPY --chown=coder initialise.sh /entrypoint.d/initialise.sh