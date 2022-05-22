package influxdb

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
)

const (
	InfluxDB = "influxdb"
)

var Command = &cli.Command{
	Name:  InfluxDB,
	Usage: "local InfluxDB server",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		local.ListCommand(InfluxDB),

		CreateCommand(),
		local.DeleteCommand(InfluxDB),

		local.LogsCommand(InfluxDB),
		local.ShellCommand(InfluxDB, "/bin/bash"),
	},
}
