# You can find the new timestamped tags here: https://hub.docker.com/r/gitpod/workspace-full/tags
FROM gitpod/workspace-go:2024-02-07-00-25-50

# Install NPM (https://github.com/gitpod-io/workspace-images/blob/main/chunks/lang-node/Dockerfile)
ENV TRIGGER_REBUILD=1
ENV NODE_VERSION=20.11.0

ENV PNPM_HOME=/home/gitpod/.pnpm
ENV PATH=/home/gitpod/.nvm/versions/node/v${NODE_VERSION}/bin:/home/gitpod/.yarn/bin:${PNPM_HOME}:$PATH

# Custom UMH
RUN curl -fsSL https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.3/install.sh | PROFILE=/dev/null bash \
    && bash -c ". .nvm/nvm.sh \
        && nvm install v${NODE_VERSION} \
        && nvm alias default v${NODE_VERSION} \
        && npm install -g typescript yarn node-gyp"

# Configure Git to skip Git LFS
RUN git config --global lfs.fetchexclude '*'

RUN curl https://cli-assets.heroku.com/install-ubuntu.sh | sh