#!/bin/bash
add-apt-repository ppa:gophers/go

apt-get update
apt-get install -y software-properties-common

# Install Docker
apt-get install -y docker.io
ln -sf /usr/bin/docker.io /usr/local/bin/docker
echo 'DOCKER_OPTS="--storage-driver=devicemapper -H tcp://0.0.0.0:2375 -H unix:///var/run/docker.sock"' > /etc/default/docker.io
service docker.io restart

# Install Go
apt-get install -y golang

# Set Go ENV variable
export GOPATH=/go
echo 'GOPATH=/go' > /etc/profile.d/go.sh
echo 'GOROOT=/go' >> /etc/profile.d/go.sh
echo 'PATH=$GOPATH/bin:$PATH' >> /etc/profile.d/go.sh

# Install check_docker
GOPATH=/go cd $GOPATH/src/github.com/newrelic/check_docker && go get ./...
