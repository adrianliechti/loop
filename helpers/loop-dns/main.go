package main

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	PodZone     string = "pod.cluster.local"
	ServiceZone string = "svc.cluster.local"
)

func main() {
	config, err := rest.InClusterConfig()

	if err != nil {
		log.WithError(err).
			Panic("unable to load config")
	}

	clientset, err := kubernetes.NewForConfig(config)

	if err != nil {
		log.WithError(err).
			Panic("unable to create client")
	}

	ctx := context.Background()

	if err := refresh(ctx, clientset); err != nil {
		log.WithError(err).
			Panic("unable to initially refresh hosts")
	}

	log.Info("hosts refreshed")

	timestamp := time.Now()

	go func() {
		for {
			watch, err := clientset.CoreV1().Services("").Watch(ctx, metav1.ListOptions{})

			if err != nil {
				time.Sleep(10 * time.Second)
				continue
			}

			c := watch.ResultChan()

			for {
				event, ok := <-c

				if !ok {
					log.Warn("service watcher failed")
					break
				}

				log.Info("received services update")

				_ = event
				timestamp = time.Now()
			}
		}
	}()

	ticker := time.NewTicker(10 * time.Second)

	go func() {
		t := timestamp

		for {
			<-ticker.C

			if timestamp.Equal(t) || t.After(timestamp) {
				log.Info("skip")
				continue
			}

			t = time.Now()

			if err := refresh(ctx, clientset); err != nil {
				log.WithError(err).
					Panic("unable to refresh hosts")
			}

			log.Info("hosts refreshed")
		}
	}()

	cmd := exec.Command("coredns")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	panic(cmd.Run())
}

func refresh(ctx context.Context, clientset *kubernetes.Clientset) error {
	mappings, err := getMappings(ctx, clientset)

	if err != nil {
		return err
	}

	if err := writeHosts("kubernetes", mappings); err != nil {
		return err
	}

	return nil
}

func getMappings(ctx context.Context, clientset *kubernetes.Clientset) (map[string]string, error) {
	result := make(map[string]string)

	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	for _, service := range services.Items {
		if service.Spec.ClusterIP == "" {
			continue
		}

		dns1 := strings.Join([]string{service.Name, service.Namespace}, ".")
		dns2 := strings.Join([]string{service.Name, service.Namespace, ServiceZone}, ".")

		result[dns1] = service.Spec.ClusterIP
		result[dns2] = service.Spec.ClusterIP

		if service.Namespace == "default" {
			result[service.Name] = service.Spec.ClusterIP
		}
	}

	return result, nil
}

func writeHosts(path string, hosts map[string]string) error {
	entries := make(map[string][]string)

	for _, ip := range hosts {
		aliases, ok := entries[ip]
		_ = ok

		for host, hostIP := range hosts {
			if hostIP != ip {
				continue
			}

			aliases = append(aliases, host)
		}

		entries[ip] = unique(aliases)
	}

	var buffer bytes.Buffer

	for ip, aliases := range entries {
		buffer.WriteString(ip + " \t" + strings.Join(aliases, " ") + " \n")
	}

	return ioutil.WriteFile(path, buffer.Bytes(), 0666)
}

func unique(e []string) []string {
	var r []string

	for _, s := range e {
		if !contains(r[:], s) {
			r = append(r, s)
		}
	}
	return r
}

func contains(e []string, c string) bool {
	for _, s := range e {
		if s == c {
			return true
		}
	}
	return false
}
