package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"syscall"
	"text/template"
	"time"

	"github.com/appcelerator/amp/pkg/docker"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

const (
	defaultTemplate         = "/etc/prometheus/prometheus.tpl"
	defaultConfiguration    = "/etc/prometheus/prometheus.yml"
	defaultHost             = docker.DefaultURL
	defaultPeriod           = 1
	dockerForMacIP          = "192.168.65.1"
	dockerEngineMetricsPort = 9323
	systemMetricsPort       = 9100 // node-exporter
	prometheusCmd           = "/bin/prometheus"
)

var prometheusArgs = []string{
	"-config.file=/etc/prometheus/prometheus.yml",
	"-storage.local.path=/prometheus",
	"-web.console.libraries=/usr/share/prometheus/console_libraries",
	"-web.console.templates=/usr/share/prometheus/consoles",
	"-alertmanager.url=http://alertmanager:9093",
}

type Inventory struct {
	Hostnames               []string
	DockerEngineMetricsPort int
	SystemMetricsPort       int
}

func update(pid int, client *docker.Docker, configurationTemplate string, configuration string) error {
	var configurationFile *os.File
	var hostnames []string
	// connect to the engine API
	if err := client.Connect(); err != nil {
		return err
	}
	filter := filters.NewArgs()
	filter.Add("name", "ingress")
	networkResources, err := client.GetClient().NetworkList(context.Background(), types.NetworkListOptions{Filters: filter})
	if err != nil {
		return err
	}
	if len(networkResources) != 1 {
		return errors.New("ingress network lookup failed")
	}
	networkId := networkResources[0].ID
	// when the vendors are updated to docker 17.06:
	//networkResource, err := client.GetClient().NetworkInspect(context.Background(), networkId, types.NetworkInspectOptions{})
	networkResource, err := client.GetClient().NetworkInspect(context.Background(), networkId, false)
	for _, peer := range networkResource.Peers {
		if peer.Name == "moby" && peer.IP == "127.0.0.1" {
			// DockerForMac
			hostnames = append(hostnames, dockerForMacIP)
		} else if peer.IP == "127.0.0.1" || peer.IP == "0.0.0.0" {
			// non addressable, let's hope the hostname is a better option
			hostnames = append(hostnames, peer.Name)
		} else {
			if _, err = net.LookupHost(peer.Name); err != nil {
				// can't resolve host, will use IP
				hostnames = append(hostnames, peer.IP)
			} else {
				hostnames = append(hostnames, peer.Name)
			}
		}
	}
	if len(hostnames) == 0 {
		return errors.New("host list is empty")
	}
	inventory := &Inventory{Hostnames: hostnames, DockerEngineMetricsPort: dockerEngineMetricsPort, SystemMetricsPort: systemMetricsPort}
	// prepare the configuration
	t := template.Must(template.New("prometheus.tpl").ParseFiles(configurationTemplate))
	configurationFile, err = os.Create(configuration)
	if err != nil {
		return err
	}
	err = t.Execute(configurationFile, inventory)
	if err != nil {
		return err
	}
	configurationFile.Close()

	// reload prometheus
	cmd := exec.Command("/usr/bin/killall", "-HUP", "prometheus")
	err = cmd.Run()
	if err != nil {
		log.Println("Prometheus reload failed, error message follows")
		return err
	}
	return nil
}

func main() {
	var client *docker.Docker
	var configuration string
	var configurationTemplate string
	var host string
	var period int32
	var prometheusPID int

	var RootCmd = &cobra.Command{
		Use:   "promctl",
		Short: "Prometheus controller",
		Long:  `Keep the Prometheus configuration up to date with swarm discovery`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Docker client init
			hasAScheme, err := regexp.MatchString(".*://.*", host)
			if err != nil {
				return err
			}
			if !hasAScheme {
				host = "tcp://" + host
			}
			hasAPort, err := regexp.MatchString(".*(:[0-9]+|sock)", host)
			if err != nil {
				return err
			}
			if !hasAPort {
				host = host + ":2375"
			}
			client = docker.NewClient(host, docker.DefaultVersion)
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
			tick := time.Tick(time.Duration(period) * time.Minute)
			time.Sleep(5 * time.Second)
			if err := update(prometheusPID, client, configurationTemplate, configuration); err != nil {
				return err
			}

		loop:
			for {
				select {
				case <-tick:
					update(prometheusPID, client, configurationTemplate, configuration)
				case sig := <-stop:
					log.Printf("%v signal trapped\n", sig)
					break loop
				}
			}
			log.Println("Stopping Prometheus")
			stopCmd := exec.Command("/usr/bin/killall", "prometheus")
			if err := stopCmd.Run(); err != nil {
				log.Println("unable to stop Prometheus, error message follows")
				return err
			}
			return nil
		},
	}

	RootCmd.PersistentFlags().StringVarP(&configuration, "config", "c", defaultConfiguration, "config file")
	RootCmd.PersistentFlags().StringVarP(&configurationTemplate, "template", "t", defaultTemplate, "template file")
	RootCmd.PersistentFlags().StringVar(&host, "host", defaultHost, "host")
	RootCmd.PersistentFlags().Int32VarP(&period, "period", "p", defaultPeriod, "reload period in minute")

	// start Prometheus
	proc := exec.Command(prometheusCmd, prometheusArgs...)
	stdout, err := proc.StdoutPipe()
	if err != nil {
		log.Fatalln(err)
	}
	stderr, err := proc.StderrPipe()
	if err != nil {
		log.Fatalln(err)
	}
	outscanner := bufio.NewScanner(stdout)
	errscanner := bufio.NewScanner(stderr)
	go func() {
		for outscanner.Scan() {
			fmt.Println(outscanner.Text())
		}
	}()
	go func() {
		for errscanner.Scan() {
			fmt.Println(errscanner.Text())
		}
	}()

	go func() {
		err := proc.Start()
		if err != nil {
			log.Fatalln(err)
		}
	}()
	if err := RootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}
