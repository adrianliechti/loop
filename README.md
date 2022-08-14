# loop - DevOps Power-Ups

## Requirements

- Connected Kubernetes cluster.
  Some features need support for LoadBalancer services

- Permission to list, create and delete resources on your cluster or at least one namespace

- Some features need a local running docker engine
  (e.g. [Docker Desktop](https://docs.docker.com/get-docker/))

- Some networking features will ask for root/sudo permissions


#### MacOS

[Homebrew](https://brew.sh)

```
brew install adrianliechti/tap/loop
```

#### Windows

[Scoop](https://scoop.sh)

```shell
scoop install https://raw.githubusercontent.com/adrianliechti/scoop-bucket/main/loop.json
```


## Current State

- Highly in Progress
- Commands, arguments and output will change


## Feature & Usage

### Cluster Applications

List applications on cluster or specified namespace

```
loop app list [--namespace <namespace>]
```

Get config information about an application

```
loop app info --name <name> [--namespace <namespace>]
```

Follow logs of all application's container(s)

```
loop app logs --name <name> [--namespace <namespace>]
```


### Cluster Network Access

This commands starts a lightwight VPN using [sshuttle](https://github.com/sshuttle/sshuttle) to route traffic to your cluster subnet.

```
loop connect [--namespace <namespace>]

namespace: namespace to create temporary helper in (default: default)
```


### Cluster Services Access

Catapult allows you to connect to cluster services using their cluster network address or dns names.
This works by temporarly modifing your hosts file (/etc/hosts) and start multiple port forwardings.

```
loop catapult [--namespace <namespace>] [--scope <scope>]

namespace: forward only services of specified namespace
scope:     scope the namespace for DNS resolutions
```


### Cluster Web UI

[Kubernetes Dashboard](https://github.com/kubernetes/dashboard) is a popular web-based admin UI.
This command allows to temporary start a preconfigured local instance to manage your cluster.

```
loop dashboard
```


### Expose local Servers

Expose local http server

```
loop expose http --port <port> --host <hostname> [--namespace <name>] [--namespace <namespace>]

port:      local tcp port to expose
hostname:  external hostname of ingress
namespace: namespace to create temporary tunnel in (default: default)
name:      name of tunnel deployment and service
```

```
loop expose tcp --port <port> [--host <hostname>] [--namespace <name>] [--namespace <namespace>]

port:      local tcp ports to expose (can be specified multiple times)
hostname:  external hostname of load balancer service (needs External-DNS)
namespace: namespace to create temporary tunnel in (default: default)
name:      name of tunnel deployment and service
```


### Remote Development Tools

Temporary start a remote [Docker](https://www.docker.com/) daemon to leverage additional compute power or build complex amd64 images on a M1 processsor (as a real world usecase).

```
loop remote docker [--namespace <namespace>]
```

Start and attach a remote shell with the current workdir mounted in /mnt/src.

```
loop remote shell [--image >image>] [--namespace <namespace>]

image:     image to use for shell
```

Start a remote VSCode Server with the current workdir mounted in /mnt/src.

```
loop remote code [--namespace <namespace>]

port:      local port to access VSCode Server Web UI
```