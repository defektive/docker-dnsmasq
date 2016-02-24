// Copyright © 2016 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
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
)

var docker *dockerclient.DockerClient

// listenCmd represents the listen command
var listenCmd = &cobra.Command{
	Use:   "listen",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {

		dc, _ := dockerclient.NewDockerClient("unix:///var/run/docker.sock", nil)
		docker = dc

		updateDNSMasq()
		docker.StartMonitorEvents(eventCallback, nil)
		waitForInterrupt()
	},
}

func init() {
	RootCmd.AddCommand(listenCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// listenCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// listenCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

}

func updateDNSMasq() {
	// Get only running containers
	containers, err := docker.ListContainers(false, false, "")
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.OpenFile("/etc/dnsmasq.d/docker.conf", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0744)
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
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Restarted DNSMasq: %q\n", out.String())
}

// Callback used to listen to Docker's events
func eventCallback(event *dockerclient.Event, ec chan error, args ...interface{}) {
	updateDNSMasq()
}

func waitForInterrupt() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	for _ = range sigChan {
		docker.StopAllMonitorEvents()
		os.Exit(0)
	}
}
