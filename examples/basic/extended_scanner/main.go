package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/go-ble/ble/examples/lib/dev"
	"log"
	"time"

	"github.com/go-ble/ble"
	"github.com/pkg/errors"
)

var (
	device = flag.String("device", "default", "implementation of ble")
	du     = flag.Duration("du", 5*time.Second, "scanning duration")
	dup    = flag.Bool("dup", true, "allow duplicate reported")
)

func main() {
	flag.Parse()

	d, err := dev.AdvertisingExtensionsDevice()
	if err != nil {
		log.Fatalf("can't new device : %s", err)
	}
	ble.SetDefaultDevice(d)
	// ExtendedScan for specified durantion, or until interrupted by user.
	fmt.Printf("ExtendedScaning for %s...\n", *du)
	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), *du))
	chkErr(ble.ExtendedScan(ctx, *dup, advHandler, nil))
}

func advHandler(a ble.ExtendedAdvertisement) {
	if a.Connectable() {
		fmt.Printf("[%s] C %3d:", a.Addr(), a.RSSI())
	} else {
		fmt.Printf("[%s] N %3d:", a.Addr(), a.RSSI())
	}
	comma := ""
	if len(a.LocalName()) > 0 {
		fmt.Printf(" Name: %s", a.LocalName())
		comma = ","
	}
	if len(a.Services()) > 0 {
		fmt.Printf("%s Svcs: %v", comma, a.Services())
		comma = ","
	}
	if len(a.ManufacturerData()) > 0 {
		fmt.Printf("%s MD: %X", comma, a.ManufacturerData())
	}
	fmt.Printf("\n")
}

func chkErr(err error) {
	switch errors.Cause(err) {
	case nil:
	case context.DeadlineExceeded:
		fmt.Printf("done\n")
	case context.Canceled:
		fmt.Printf("canceled\n")
	default:
		log.Fatalf(err.Error())
	}
}
