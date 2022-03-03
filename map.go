package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ammario/ipisp"
	"github.com/davecgh/go-spew/spew"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-mapper/traceroute"
	"github.com/gagliardetto/streamject"
	. "github.com/gagliardetto/utilz"
	"github.com/go-ping/ping"
	"github.com/hako/durafmt"
	"github.com/muesli/cache2go"
	"github.com/urfave/cli"
)

func newMapCommand() cli.Command {
	return cli.Command{
		Name:        "map",
		Description: "Map solana nodes.",
		Before: func(c *cli.Context) error {
			return nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "rpc",
				Usage: "RPC endpoint node to use.",
				Value: rpc.MainNetBeta_RPC,
			},
			&cli.StringFlag{
				Name:  "dir",
				Usage: "Directory where to save the result file.",
				Value: ".",
			},
			&cli.Int64Flag{
				Name:  "c",
				Usage: "Concurrency",
				Value: 10,
			},
			&cli.IntFlag{
				Name:  "ping-count",
				Usage: "Ping how many times",
				Value: 25,
			},
			&cli.IntFlag{
				Name:  "limit",
				Usage: "How many nodes to map.",
				Value: 0,
			},
		},
		Action: func(c *cli.Context) error {
			// ulimit -n 65536
			ul := getRlimit()
			if ul.Cur < 10000 {
				return fmt.Errorf("your ulimit is low (%v); use `ulimit -n 65536` to increase it", ul.Cur)
			}

			if !isLikelyRoot() {
				Ln("You're (most likely) not running this program as a privileged user;")
				Ln("To run traceroute, you need privileges.")
				yes, err := CLIAskYesNo("Do you want to continue? (even though it most likely won't work)")
				if err != nil {
					panic(err)
				}
				if !yes {
					Ln("Exiting.")
					return nil
				}
			}
			took := NewTimerRaw()
			mainDir := c.String("dir")
			concurrency := c.Int64("c")
			pingCount := c.Int("ping-count")
			limit := c.Int("limit")

			var (
				rpcEndpoint string = c.String("rpc")
			)

			resultFilepath := filepath.Join(mainDir, Sf("mainnet-beta_%s.json", time.Now().Format(FilenameTimeFormat)))
			defer func() {
				Sfln("Results successfully saved to: %s", MustAbs(resultFilepath))
			}()
			stj, err := streamject.New(resultFilepath)
			if err != nil {
				panic(err)
			}

			client := rpc.New(rpcEndpoint)

			nodes, err := client.GetClusterNodes(
				context.TODO(),
			)
			if err != nil {
				panic(err)
			}
			stake, err := client.GetVoteAccounts(
				context.TODO(),
				nil,
			)
			if err != nil {
				panic(err)
			}
			Sfln("Results will be saved to %s", MustAbs(resultFilepath))
			Sfln("Concurrency: %v", concurrency)
			Sfln("Got %v nodes from RPC", len(nodes))
			Sfln(
				"Got %v stake (%v current + %v deliquent)",
				len(stake.Current)+len(stake.Delinquent),
				len(stake.Current),
				len(stake.Delinquent),
			)

			eg := NewSizedGroup(concurrency)
			for nodeIndex := range nodes {
				if limit > 0 && nodeIndex >= limit {
					break
				}
				node := nodes[nodeIndex]
				Sfln("Processing node %v/%v", nodeIndex+1, len(nodes))

				eg.Go(func() error {
					err := func(node *rpc.GetClusterNodesResult) error {
						if node.TPU == nil && node.Gossip == nil {
							return fmt.Errorf("node.TPU and node.Gossip are nil: %s", spew.Sprint(node))
						}

						var hostPort string
						if node.Gossip != nil {
							hostPort = *node.Gossip
						}
						if node.TPU != nil {
							hostPort = *node.TPU
						}
						host, _, err := net.SplitHostPort(hostPort)
						if err != nil {
							return fmt.Errorf(
								"err while SplitHostPort: %s %s",
								err,
								spew.Sprint(node),
							)
						}

						var asn *ipisp.Response
						errs := RetryExponentialBackoff(3, time.Millisecond*500, func() (e error) {
							asn, e = ASNLookupIP(net.ParseIP(host))
							return e
						})
						if len(errs) > 0 {
							return fmt.Errorf(
								"err while ASNLookupIP: %s %s",
								CombineErrors(errs...),
								spew.Sprint(node),
							)
						}

						pinger, err := ping.NewPinger(host)
						if err != nil {
							return err
						}
						pinger.Count = pingCount
						pinger.Timeout = time.Second * 25
						err = pinger.Run() // Blocks until finished.
						if err != nil {
							return err
						}
						stats := pinger.Statistics()

						nodeInfo := NodeInfo{
							Node:  node,
							ASN:   asn,
							Ping:  stats,
							Stake: getByNodeID(stake, node.Pubkey),
						}

						{
							var m = traceroute.DEFAULT_MAX_HOPS
							var f = traceroute.DEFAULT_FIRST_HOP
							var q = 1

							options := traceroute.TracerouteOptions{}
							options.SetRetries(q - 1)
							options.SetMaxHops(m + 1)
							options.SetFirstHop(f)

							ipAddr, err := net.ResolveIPAddr("ip", host)
							if err != nil {
								Errorf("err resolving traceroute target: %s", err)
							} else {
								fmt.Printf("traceroute to %v (%v), %v hops max, %v byte packets\n", host, ipAddr, options.MaxHops(), options.PacketSize())

								c := make(chan traceroute.TracerouteHop, 0)
								go func() {
									for {
										hop, ok := <-c
										if !ok {
											return
										}
										{
											RetryExponentialBackoff(3, time.Millisecond*500, func() (e error) {
												hop.ASN, e = ASNLookupIP(hop.Address[:])
												return e
											})
										}
										nodeInfo.Traceroute = append(nodeInfo.Traceroute, hop)
										// printHop(hop)
									}
								}()

								_, err = traceroute.Traceroute(host, &options, c)
								if err != nil {
									return fmt.Errorf("error while doing traceroute: %w", err)
								}
							}
						}
						// spew.Dump(nodeInfo)

						return stj.Append(nodeInfo)
					}(node)
					if err != nil {
						e := fmt.Errorf("error while processing %s node: %w", node.Pubkey, err)
						Errorf("%s", e)
						return e
					}

					return nil
				})
			}

			if err := eg.Wait(); err != nil {
				panic(err)
			}
			// TODO:
			// - cuncurrent test: slot latency on ws subscription.
			Successf("Done. Processed %v nodes. Took %s", len(nodes), durafmt.Parse(took()))
			return nil
		},
	}
}

func isLikelyRoot() bool {
	if os.Geteuid() == 0 {
		return true
	}
	if os.Getenv("SUDO_UID") != "" {
		return true
	}
	if os.Getenv("SUDO_GID") != "" {
		return true
	}
	if os.Getenv("SUDO_USER") != "" {
		return true
	}
	return false
}

func getRlimit() syscall.Rlimit {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		panic(fmt.Errorf("error getting rlimit: %s", err))
	}
	return rLimit
}

func getByNodeID(stake *rpc.GetVoteAccountsResult, nodeID solana.PublicKey) *rpc.VoteAccountsResult {
	for i := range stake.Current {
		res := stake.Current[i]
		if res.NodePubkey.Equals(nodeID) {
			return &res
		}
	}
	for i := range stake.Delinquent {
		res := stake.Delinquent[i]
		if res.NodePubkey.Equals(nodeID) {
			return &res
		}
	}
	return nil
}

var (
	ASNInfoGetter    ipisp.Client
	asnLookupIPCache *cache2go.CacheTable = cache2go.Cache("asnLookupIPCache")
)

func init() {
	var err error
	ASNInfoGetter, err = ipisp.NewDNSClient()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
}

func ASNLookupIP(ip net.IP) (*ipisp.Response, error) {
	cacheKey := ip.String()
	{ // check cache:
		// Let's retrieve the item from the cache.
		cached, err := asnLookupIPCache.Value(cacheKey)
		if err == nil {
			return cached.Data().(*ipisp.Response), nil
		}
	}

	res, err := ASNInfoGetter.LookupIP(ip)
	if err == nil {
		asnLookupIPCache.Add(cacheKey, time.Minute, res)
	}
	return res, err
}
