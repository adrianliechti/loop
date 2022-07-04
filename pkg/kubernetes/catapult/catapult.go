package catapult

import (
	"context"
	"errors"

	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/system"

	"github.com/ChrisWiegman/goodhosts/v4/pkg/goodhosts"
	"github.com/hashicorp/go-multierror"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CatapultOptions struct {
	Scope     string
	Namespace string

	Selector string
}

func Start(ctx context.Context, client kubernetes.Client, options CatapultOptions) error {
	list, err := client.CoreV1().Services(options.Namespace).
		List(ctx, metav1.ListOptions{
			LabelSelector: options.Selector,
		})

	if err != nil {
		return err
	}

	var services []*CatapultService

	for _, s := range list.Items {
		if isHidden(s.Namespace) {
			continue
		}

		service, err := NewService(client, options, s)

		if err != nil {
			continue
		}

		services = append(services, service)
	}

	if len(services) == 0 {
		return errors.New("no services found by filter")
	}

	hostsfile, err := goodhosts.NewHosts("Loop")

	if err != nil {
		return err
	}

	defer func() {
		hostsfile.Load()
		hostsfile.RemoveSection()
		hostsfile.Flush()
	}()

	var errsetup error

	for _, service := range services {
		tunnels, err := service.Tunnels(ctx)

		if err != nil {
			errsetup = multierror.Append(errsetup, err)
			continue
		}

		for _, tunnel := range tunnels {
			address, err := tunnel.Address()

			if err != nil {
				errsetup = multierror.Append(errsetup, err)
				continue
			}

			if err := hostsfile.Add(address, "", tunnel.Hosts()...); err != nil {
				errsetup = multierror.Append(errsetup, err)
				continue
			}

			if err := system.AliasIP(ctx, address); err != nil {
				errsetup = multierror.Append(errsetup, err)
				continue
			}

			defer system.UnaliasIP(context.Background(), address)

			if err := tunnel.Start(ctx, nil); err != nil {
				errsetup = multierror.Append(errsetup, err)
				continue
			}
		}
	}

	if err := hostsfile.Flush(); err != nil {
		errsetup = multierror.Append(errsetup, err)
	}

	if errsetup != nil {
		return errsetup
	}

	<-ctx.Done()
	return nil
}

func isHidden(namespace string) bool {
	return false
}
