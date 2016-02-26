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
	"os"
	"time"
)


var docker *dockerclient.DockerClient
var updated = true
var dockerSocketPath string
var dnsmasqConfigPath string
// daemonCmd represents the daemon command
var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Update dnsmasq config when containers start and stop.",
	Long: `Listen to docker events. Add/remove dnsmasq entries when containers
	start or stop. Then restart dnsmasq. `,
	Run: func(cmd *cobra.Command, args []string) {

		dc, _ := dockerclient.NewDockerClient(dockerSocketPath, nil)
		docker = dc

		updateDNSMasq()
		docker.StartMonitorEvents(eventCallback, nil)


		ticker := time.NewTicker(5 * time.Second)
		quit := make(chan struct{})
		go func() {
		    for {
		       select {
		        case <- ticker.C:
		            // do stuff
								if(updated) {
									updateDNSMasq()
								}
		        case <- quit:
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
	RootCmd.PersistentFlags().StringVarP(&dockerSocketPath, "docker-socket", "d", "unix:///var/run/docker.sock", "path to docker socket")
	RootCmd.PersistentFlags().StringVarP(&dnsmasqConfigPath, "dnsmasq-config", "c", "/etc/dnsmasq.d/docker.conf", "path to dnsmasq config file (this file should be empty)")

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

func restartDNS() {
	cmd := exec.Command("systemctl", "restart", "dnsmasq")
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Restarted DNSMasq\n")
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
