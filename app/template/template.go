package template

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

type template string

var (
	TemplateAngular template = "angular"
	TemplateASPNET  template = "aspnet"
	TemplateGolang  template = "golang"
	TemplateNginx   template = "nginx"
	TemplatePack    template = "pack"
	TemplatePython  template = "python"
	TemplateReact   template = "react"
	TemplateSpring  template = "spring"
)

var Command = &cli.Command{
	Name:  "template",
	Usage: "create new applications from template",

	HideHelpCommand: true,

	Category: app.CategoryUtilities,

	Subcommands: []*cli.Command{
		{
			Name:  "react",
			Usage: "create React web app",

			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "name",
					Usage:    "package name",
					Required: true,
				},
			},

			Action: func(c *cli.Context) error {
				options := templateOptions{
					Name: c.String("name"),
				}

				return runTemplate(c.Context, "", TemplateReact, options)
			},
		},
		{
			Name:  "angular",
			Usage: "create Angular app",

			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "name",
					Usage:    "package name",
					Required: true,
				},
			},

			Action: func(c *cli.Context) error {
				options := templateOptions{
					Name: c.String("name"),
				}

				return runTemplate(c.Context, "", TemplateAngular, options)
			},
		},
		{
			Name:  "golang",
			Usage: "create Go web app",

			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "name",
					Usage:    "module name",
					Required: true,
				},
			},

			Action: func(c *cli.Context) error {
				options := templateOptions{
					Name: c.String("name"),
				}

				return runTemplate(c.Context, "", TemplateGolang, options)
			},
		},
		{
			Name:  "python",
			Usage: "create Python web app",

			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "name",
					Usage:    "app name",
					Required: true,
				},
			},

			Action: func(c *cli.Context) error {
				options := templateOptions{
					Name: c.String("name"),
				}

				return runTemplate(c.Context, "", TemplatePython, options)
			},
		},
		{
			Name:  "spring",
			Usage: "create Java Spring web app",

			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "group",
					Usage:    "application group",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "name",
					Usage:    "application name",
					Required: true,
				},
			},

			Action: func(c *cli.Context) error {
				options := templateOptions{
					Group: c.String("group"),
					Name:  c.String("name"),
				}

				return runTemplate(c.Context, "", TemplateSpring, options)
			},
		},
		{
			Name:  "aspnet",
			Usage: "create ASP.NET Core app",

			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "name",
					Usage:    "application name",
					Required: true,
				},
			},

			Action: func(c *cli.Context) error {
				options := templateOptions{
					Name: c.String("name"),
				}

				return runTemplate(c.Context, "", TemplateASPNET, options)
			},
		},

		{
			Name:  "nginx",
			Usage: "create Nginx web app",

			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "name",
					Usage:    "app name",
					Required: true,
				},
			},

			Action: func(c *cli.Context) error {
				options := templateOptions{
					Name: c.String("name"),
				}

				return runTemplate(c.Context, "", TemplateNginx, options)
			},
		},
		{
			Name:  "pack",
			Usage: "create app using buildpacks",

			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "name",
					Usage:    "app name",
					Required: true,
				},
			},

			Action: func(c *cli.Context) error {
				options := templateOptions{
					Name: c.String("name"),
				}

				return runTemplate(c.Context, "", TemplatePack, options)
			},
		},
	},
}

func runTemplate(ctx context.Context, path string, template template, options templateOptions) error {
	if options.Name == "" {
		options.Name = "demo"
	}

	if options.Version == "" {
		options.Version = "1.0.0"
	}

	path, err := app.EmptyDir(path, options.Name)

	if err != nil {
		return err
	}

	runImage := fmt.Sprintf("adrianliechti/loop-template:%s", template)

	runOptions := docker.RunOptions{
		Env: options.env(),
		Volumes: map[string]string{
			path: "/src",
		},
	}

	return docker.RunInteractive(ctx, runImage, runOptions)
}

type templateOptions struct {
	Group   string
	Name    string
	Version string

	Host string

	EnableIngress     bool
	EnablePersistence bool
}

func (o *templateOptions) env() map[string]string {
	name := o.Name
	version := o.Version

	group := strings.ToLower(o.Group)
	artifact := strings.ToLower(name)

	chart := strings.ToLower(name)
	chartVersion := strings.ToLower(version)

	image := strings.ToLower(name)
	imageTag := strings.ToLower(version)

	host := strings.ToLower(o.Host)

	ingress := strconv.FormatBool(o.EnableIngress)
	persistent := strconv.FormatBool(o.EnablePersistence)

	result := map[string]string{
		"APP_NAME": name,

		"APP_GROUP":    group,
		"APP_ARTIFACT": artifact,

		"CHART_NAME":    chart,
		"CHART_VERSION": chartVersion,

		"IMAGE_REPOSITORY": image,
		"IMAGE_TAG":        imageTag,

		"APP_HOSTNAME": host,

		"APP_INGRESS":    ingress,
		"APP_PERSISTENT": persistent,
	}

	return result
}
