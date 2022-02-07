#!/bin/bash

pushd $(git rev-parse --show-toplevel) > /dev/null

for directory in ./deployment/*/ ; do

  DOCKER_FILE_PATH="${directory}Dockerfile"

  if [ -f "$DOCKER_FILE_PATH" ]; then
    echo "Building $directory"
    docker build -f $DOCKER_FILE_PATH .

    if [ $? -ne 0 ]; then
        echo "Build failed for $directory"
        exit 1
    fi
  fi
done

popd > /dev/null