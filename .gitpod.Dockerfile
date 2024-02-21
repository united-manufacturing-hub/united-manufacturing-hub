FROM gitpod/workspace-full
# Configure Git to skip Git LFS
RUN git config --global lfs.fetchexclude '*'