#!/bin/bash

set -e

# Get the full path to the script, resolving symlinks if necessary
SCRIPT=$(readlink -f "$0")

# Extract the directory part of the path
SCRIPT_DIR=$(dirname "$SCRIPT")

cd $SCRIPT_DIR

mkdir -p $SCRIPT_DIR/bin

echo "Building the container"
# Build the docker image
docker build -t disk-usage:latest --no-cache .

echo "Starting the container"
# Run the container to execute the build
docker run --rm -v "${PWD}/bin:/app/host" disk-usage:latest /bin/bash -c "cp -r /app/bin/* /app/host"

echo "Make the binaries executable"
# Run the container to execute the build
# The binaries will be in the ./bin directory
chmod +x "$SCRIPT_DIR/bin/disk-usage-linux-amd64"
chmod +x "$SCRIPT_DIR/bin/disk-usage-darwin-amd64"
chmod +x "$SCRIPT_DIR/bin/disk-usage-darwin-arm64"

# run a separate build for Ubuntu 20.04 since it uses an old version of glibc. @TODO: Remove when we no longer use 20.04
echo "Building the Ubuntu 20.04 container"
docker build -f ./Dockerfile.ubuntu.20.04 -t disk-usage:ubuntu-20-04 --no-cache .
echo "Starting the Ubuntu 20.04 container"
docker run --rm -v "${PWD}/bin:/app/host" disk-usage:ubuntu-20-04 /bin/bash -c "cp -r /app/bin/* /app/host"
chmod +x "$SCRIPT_DIR/bin/disk-usage-ubuntu-20-04"

# create a symbolic link to the correct executable based on the current OS

# first remove the current link
if [ -L "$SCRIPT_DIR/bin/disk-usage" ]; then
  echo "File exists."
  rm "$SCRIPT_DIR/bin/disk-usage"
fi

# Determine the operating system
OS=$(uname)

# Determine the architecture
ARCH=$(uname -m)

if [ "$OS" == "Linux" ]; then
  # Command for Linux
  echo "Running on Linux"
  ln -s "$SCRIPT_DIR/bin/disk-usage-linux-amd64" "$SCRIPT_DIR/bin/disk-usage"

elif [ "$OS" == "Darwin" ]; then
  # macOS
  echo "Running on macOS"

  if [[ "$ARCH" == "x86_64" ]]; then
    # Intel-based macOS
    echo "Intel-based Mac"
    ln -s "$SCRIPT_DIR/bin/disk-usage-darwin-amd64" "$SCRIPT_DIR/bin/disk-usage"

  elif [[ "$ARCH" == "arm64" ]]; then
    # Apple Silicon macOS
    echo "Apple Silicon Mac"
    ln -s "$SCRIPT_DIR/bin/disk-usage-darwin-arm64" "$SCRIPT_DIR/bin/disk-usage"
  fi
fi
