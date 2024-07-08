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


WORKDIR /home/coder

ARG NODE_VERSION=20.x

RUN curl -sL https://deb.nodesource.com/setup_${NODE_VERSION} -o /tmp/nodesource_setup.sh && \
    sudo bash /tmp/nodesource_setup.sh && \
    sudo apt-get install -y nodejs && \
    sudo apt-get clean && \
    sudo apt-get auto-remove -y && \
    sudo rm -rf /var/cache/apt/* && \
    sudo rm -rf /var/lib/apt/lists/* && \
    sudo rm -rf /tmp/*

RUN sudo npm install -g npm

RUN git config --global --add safe.directory /workspaces
RUN git config --global user.email "supercoder@superagi.com"
RUN git config --global user.name "SuperCoder"

RUN mkdir -p /home/coder/.config/code-server/
COPY --chown=coder config.yaml /home/coder/.config/code-server/config.yaml
COPY --chown=coder settings.json /home/coder/.local/share/code-server/User/settings.json
