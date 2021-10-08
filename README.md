# DevOps Loop Power-Ups

## Requirements

- Connected Kubernetes cluster.
  Some features need support for LoadBalancer services

- Permission to list, create and delete resources on your cluster or at least one namespace

- Some features need a local running docker engine
  (e.g. [Docker Desktop](https://docs.docker.com/get-docker/))

- Some networking features will ask for root/sudo permissions


## Install

Homebrew

```
brew install adrianliechti/tap/loop
```


## Current State

- Highly in Progress
- Commands, arguments and output will change


## Feature & Usage

### Cluster Config

Import and merge a Kubernetes config file

```
loop config import -f path/to/config
```

List available config contexts or switch default context

```
loop config context [<context>]
```

List available namespaces or set default namespace

```
loop config namespace [<namespace>]
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


### Applications

List applications on cluster or specified namespace

```
loop app list [--namespace <namespace>]

namespace: application namespace
```

Get config information about an application

```
loop app info --name <namespace> [--namespace <namespace>]

namespace: application namespace
```

Follow logs of all application's container(s)

```
loop app logs --name <namespace> [--namespace <namespace>]

namespace: application namespace
```


### Application Utilities

Count lines of source code and comments in the current workdir

```
loop app cloc
```

Create a runnable app image from source code in workdir. The command useses [Cloud Native Buildpacks
](https://buildpacks.io/) and the [Google Builder & Buildpacks](https://github.com/GoogleCloudPlatform/buildpacks) by default

```
loop pack --image <image> [--builder <builder image>]

image:   application image
builder: builder image
```


### Kubernetes Dashboard

[Kubernetes Dashboard](https://github.com/kubernetes/dashboard) is a popular web-based admin UI.
This command allows to temporary start a preconfigured local instance to manage your cluster.

```
loop dashboard
```


### Remote Docker Daemon

Temporary start a remote [Docker](https://www.docker.com/) daemon to leverage additional compute power or build complex amd64 images on a M1 processsor (as a real world usecase).

```
loop docker connect [--namespace <namespace>]

namespace: namespace to create temporary daemon in (default: default)
```


### Docker Image Analyzers & Utilities

Browse an explore image layers using [dive](https://github.com/wagoodman/dive)

```
loop docker browse --image <image>[:<tag>]
```

Lint images for security and configuration issues using [dockle](https://github.com/goodwithtech/dockle)

```
loop docker lint --image <image>[:<tag>]
```

Export Dockerfiles from images using
[whaler](https://github.com/P3GLEG/Whaler)

```
loop docker analyze --image <image>[:<tag>]
```

Scan for vulnerability and configuration issue using
[trivy](https://github.com/aquasecurity/trivy)

```
loop docker scan --image <image>[:<tag>]
```
