**docker-dnsmasq** is a tool for adding docker containers to dnsmasq config in realtime.

# Requirements

### macOS

* golang
* brew
* dnsmasq (installed with brew)
* docker-toolbox (not docker for mac)

### Linux

* golang
* systemctl
* dnsmasq
* docker

# Installation

```bash
go get github.com/defektive/docker-dnsmasq
```
# Usage

### macOS

```bash
    sudo docker-dnsmasq -c=/usr/local/etc/dnsmasq.d/docker.conf \
    -r="brew services restart dnsmasq" \
    -d=tcp://192.168.99.100:2376 \
    -t=$DOCKER_CERT_PATH daemon
```

### Linux

```bash
    sudo docker-dnsmasq daemon
```
is equal to

```bash
    sudo docker-dnsmasq -c=/etc/dnsmasq.d/docker.conf -r="systemctl restart dnsmasq" daemon
```