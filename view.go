package main

import (
	"errors"
	"sort"

	"github.com/gagliardetto/streamject"
	. "github.com/gagliardetto/utilz"
	"github.com/hako/durafmt"
	"github.com/urfave/cli"
)

func newViewCommand() cli.Command {
	return cli.Command{
		Name:        "view",
		Description: "View results file.",
		Before: func(c *cli.Context) error {
			return nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "file",
				Usage: "Results file to view.",
				Value: ".",
			},
			&cli.IntFlag{
				Name:  "top",
				Usage: "Print top X results.",
				Value: 0,
			},
			&cli.Float64Flag{
				Name:  "max-loss",
				Usage: "Exlude results that had packet loss % greates than this during ping (0 - 100).",
				Value: 5,
			},
		},
		Action: func(c *cli.Context) error {
			took := NewTimerRaw()
			defer func() {
				Sfln("took %s", took())
			}()
			filePath := c.Args().First()
			if filePath == "" {
				return errors.New("file not provided")
			}
			top := c.Int("top")
			maxLoss := c.Float64("max-loss")

			stj, err := streamject.New(filePath)
			if err != nil {
				panic(err)
			}
			if top > int(stj.NumLines()) {
				top = int(stj.NumLines())
			}
			nodes := make([]*NodeInfo, 0)
			stj.Iterate(func(line streamject.Line) bool {
				var node NodeInfo
				err := line.Decode(&node)
				if err != nil {
					panic(err)
				}
				// Exclude nodes with high packet loss:
				if node.Ping.PacketLoss > maxLoss {
					return true
				}
				// Exclude nodes without a TPU:
				// if node.Node.TPU == nil {
				// 	return true
				// }
				nodes = append(nodes, &node)
				return true
			})
			sort.Slice(nodes, func(i, j int) bool {
				return nodes[i].Ping.AvgRtt < nodes[j].Ping.AvgRtt
			})

			if top > 0 {
				nodes = nodes[:top]
			}
			for index, node := range nodes {
				if node == nil {
					continue
				}
				Sfln(
					"#%v:: %s %s (%s) -> %s (%s) (%v hops, %v success, %v named)",
					index,
					CustomConstantLength(15, node.ASN.IP.String()),
					Lime(CustomConstantLength(20, node.ASN.Name.String())),
					node.ASN.Country,
					CustomConstantLength(40, durafmt.Parse(node.Ping.AvgRtt).String()),
					formatLoss(node.Ping.PacketLoss),
					len(node.Traceroute),
					countHopsSuccess(node.Traceroute),
					countHopsNamed(node.Traceroute),
				)
			}

			return nil
		},
	}
}

func formatLoss(loss float64) string {
	if loss == 0 {
		return "no loss"
	}
	return Sf(Orange("%v%% loss"), loss)
}
