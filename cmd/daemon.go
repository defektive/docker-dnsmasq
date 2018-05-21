// Copyright © 2016 defektive <sirbradleyd@gmail.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cmd

import (
	"fmt"
	"log"
	"github.com/samalba/dockerclient"
	"github.com/spf13/cobra"
	"os/exec"
	"regexp"
	"strings"
	"os/signal"
	"syscall"
	"time"
	"os"
	"runtime"
	"net"
	"crypto/tls"
)

var docker *dockerclient.DockerClient
var updated = true
var dockerSocketPath string
var dnsmasqConfigPath string
var dnsmasqRestartCommand string
var dockerMachineIp string
var dockerTlsPath string
// daemonCmd represents the daemon command
var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Update dnsmasq config when containers start and stop.",
	Long: `Listen to docker events. Add/remove dnsmasq entries when containers
	start or stop. Then restart dnsmasq. `,
	Run: func(cmd *cobra.Command, args []string) {
		var cert *tls.Config
		var err error
		if dockerTlsPath!=""{
			cert, err = dockerclient.TLSConfigFromCertPath(dockerTlsPath)
			if err != nil {
				log.Fatal(err)
			}
		}
		dc, _ := dockerclient.NewDockerClient(dockerSocketPath, cert)
		docker = dc

		updateDNSMasq()
		docker.StartMonitorEvents(eventCallback, nil)

		ticker := time.NewTicker(5 * time.Second)
		quit := make(chan struct{})
		go func() {
			for {
				select {
				case <-ticker.C:
					// do stuff
					if (updated) {
						updateDNSMasq()
					}
				case <-quit:
					ticker.Stop()
					return
				}
			}
		}()

		waitForInterrupt()
	},
}

func init() {
	RootCmd.AddCommand(daemonCmd)
	daemonCmd.PersistentFlags().StringVarP(&dockerSocketPath, "docker-socket", "d", "unix:///var/run/docker.sock", "path to docker socket")
	daemonCmd.PersistentFlags().StringVarP(&dnsmasqConfigPath, "dnsmasq-config", "c", "/etc/dnsmasq.d/docker.conf", "path to dnsmasq config file (this file should be empty)")
	daemonCmd.PersistentFlags().StringVarP(&dnsmasqRestartCommand, "daemon-restart", "r", "systemctl restart dnsmasq", "command to restart dnsmasq")
	daemonCmd.PersistentFlags().StringVarP(&dockerMachineIp, "docker-machine-ip", "m", "192.168.99.100", "set ip of docker machine")
	daemonCmd.PersistentFlags().StringVarP(&dockerTlsPath, "docker-tls-path", "t", "", "set tls path for docker api")
}
func defaultDockerMachineIp() string {
	output, _ := exec.Command("docker-machine", "ip").Output()
	return strings.TrimSuffix(string(output), "\n")
}
func updateDNSMasq() {
	// Get only running containers
	containers, err := docker.ListContainers(false, false, "")
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.OpenFile(dnsmasqConfigPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0744)
	if err != nil {
		panic(err)
	}

	for _, c := range containers {
		f.WriteString(dnsmasqConfig(c))
		if runtime.GOOS == "darwin" {
			addDockerRoute(c)
		}
	}

	f.Close()
	restartDNS()
}

func containerDomain(container dockerclient.Container) string {
	info, _ := docker.InspectContainer(container.Id)
	regex, _ := regexp.Compile(`VIRTUAL_HOST=([^\s]+)`)
	res := regex.FindStringSubmatch(strings.Join(info.Config.Env, " "))

	domain := ""
	if res != nil {
		domain = res[1]
	}
	return domain
}

func containerIP(container dockerclient.Container) string {
	var ip string
	for _, n := range container.NetworkSettings.Networks {
		ip = n.IPAddress
		break
	}
	return ip
}

func dnsmasqConfig(container dockerclient.Container) string {
	ip := containerIP(container)
	domain := containerDomain(container)

	if ip != "" && domain != "" {
		return fmt.Sprintf("address=/%s/%s\n", domain, ip)
	}

	return ""
}

func addDockerRoute(container dockerclient.Container) {
	subnet := net.ParseIP(containerIP(container)).Mask(net.CIDRMask(16, 32)).String()
	err := exec.Command("/bin/sh", "-c", fmt.Sprintf("route delete %s/16 %s", subnet, dockerMachineIp)).Run()
	if err != nil {
		log.Fatal(err)
	}
	err = exec.Command("/bin/sh", "-c", fmt.Sprintf("route add %s/16 %s", subnet, dockerMachineIp)).Run()
	if err != nil {
		log.Fatal(err)
	}
	log.Println(fmt.Sprintf("added %s/16 -> %s", subnet, dockerMachineIp))
}

func restartDNS() {
	cmd := exec.Command("/bin/sh", "-c", dnsmasqRestartCommand)
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("restarted DNSMasq")
	updated = false
}

func eventCallback(event *dockerclient.Event, ec chan error, args ...interface{}) {
	updated = true
}

func waitForInterrupt() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	for _ = range sigChan {
		docker.StopAllMonitorEvents()
		os.Exit(0)
	}
}
