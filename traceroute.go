package main

import (
	"fmt"

	"github.com/gagliardetto/solana-mapper/traceroute"
)

func countHopsSuccess(hops []traceroute.TracerouteHop) (success int) {
	for _, hop := range hops {
		if hop.Success {
			success++
		}
	}
	return success
}

func countHopsNamed(hops []traceroute.TracerouteHop) (named int) {
	for _, hop := range hops {
		if hop.Host != "" {
			named++
		}
	}
	return named
}

func printHop(hop traceroute.TracerouteHop) {
	addr := fmt.Sprintf("%v.%v.%v.%v", hop.Address[0], hop.Address[1], hop.Address[2], hop.Address[3])
	hostOrAddr := addr
	if hop.Host != "" {
		hostOrAddr = hop.Host
	}
	if hop.Success {
		fmt.Printf("%-3d %v (%v)  %v\n", hop.TTL, hostOrAddr, addr, hop.ElapsedTime)
	} else {
		fmt.Printf("%-3d *\n", hop.TTL)
	}
}

func address(address [4]byte) string {
	return fmt.Sprintf("%v.%v.%v.%v", address[0], address[1], address[2], address[3])
}
