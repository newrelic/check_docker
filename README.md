check_docker
============

A Nagios check to check some basic statistics reported by the Docker daemon.
Additionally validates the absence of Ghost containers and may optionally
be made to require the presence of a container running from a particular image
tag.

`check_docker` is written in Go and is multi-threaded to keep the
drag time low on your Nagios server. It makes two API requests to the
Docker daemon, one to `/info` and one to `/containers/json`
and processes the results, all simultaneously.

It is built using the the
[go_nagios](http://github.com/newrelic/go_nagios)
framework.

Installing
----------
If you would rather not build the binaries yourself, you can install compiled,
statically-linked [binaries](https://github.com/newrelic/check_docker/releases)
for Linux or MacOSX. Simply download the tarball, extract it and use the binary
of your choice.

Building
--------
```
go get github.com/newrelic/go_nagios
go build
```

Usage
-----
```
Usage of ./check_docker:
  -base-url="http://docker-server:2375": The Base URL for the Docker server
  -warn-data-space=100: Warning threshold for Data Space
  -crit-data-space=100: Critical threshold for Data Space
  -warn-meta-space=100: Warning threshold for Metadata Space
  -crit-meta-space=100: Critical threshold for Metadata Space
  -image-id="": An image ID that must be running on the Docker server
  -tls-cert="": Path to TLS cert file
  -tls-key="": Path to TLS key file
  -tls-ca="": Path to TLS CA file
```

`-base-url`: Here you specify the base url of the docker server.

`-image-id`: You can specify an image tag that needs to be running on the server for
certain cases where you have pegged a container to a server (e.g. each server
has a Nagios monitoring container running to report on server health). Will not
require any particular image if left off.

`-(warn|crit)-(meta|data)-space`: the thresholds at which the named Nagios status codes
should be emitted. These are percentages, so `-crit-data-space=95` would send
a CRITICAL response when the threshold of 95% is crossed. Defaults are 100%.

Contributions
-------------

Contributions are more than welcome. Bug reports with specific reproduction
steps are great. If you have a code contribution you'd like to make, open a
pull request with suggested code.

Pull requests should:

 * Clearly state their intent in the title
 * Have a description that explains the need for the changes
 * Include tests!
 * Not break the public API

Testing for Contributors
------------------------

`go test` can be run on 2 different platforms:

1. In Darwin(aufs), assuming you already setup Boot2Docker:
    ```
    cd $GOPATH/src/github.com/newrelic/check_docker
    docker run -d busybox /bin/sh -c 'while true; do echo hello world; sleep 1; done'
    export DOCKER_IMAGE=$(docker ps | grep busybox | awk '{print $2}')

    cd $GOPATH/src/github.com/newrelic/check_docker
    go get ./... && go test
    ```

2. In Linux(devicemapper), you are running the tests inside vagrant:
    ```
    cd $GOPATH/src/github.com/newrelic/check_docker
    vagrant up --provider virtualbox
    vagrant ssh

    # Inside Vagrant
    sudo docker run -d busybox /bin/sh -c 'while true; do echo hello world; sleep 1; done'
    export DOCKER_IMAGE=$(sudo docker ps | grep busybox | awk '{print $2}')

    export GOPATH=/go
    cd $GOPATH/src/github.com/newrelic/check_docker
    go get ./... && go test
    ```


Copyright (c) 2014 New Relic, Inc. All rights reserved.
