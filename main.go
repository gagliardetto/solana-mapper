// Solana nodes mapper.
package main

import (
	"log"
	"os"
	"sort"

	"github.com/ammario/ipisp"
	"github.com/go-ping/ping"
	"github.com/urfave/cli"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-mapper/traceroute"
)

type NodeInfo struct {
	Node       *rpc.GetClusterNodesResult
	ASN        *ipisp.Response
	Ping       *ping.Statistics
	Stake      *rpc.VoteAccountsResult
	Traceroute []traceroute.TracerouteHop
}

func main() {
	app := &cli.App{
		Name:        "mapper",
		Version:     "v0.0.1",
		Description: "Solana node info mapper.",
		Before: func(c *cli.Context) error {
			return nil
		},
		Flags:  []cli.Flag{},
		Action: nil,
		Commands: []cli.Command{
			newMapCommand(),
			newViewCommand(),
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
