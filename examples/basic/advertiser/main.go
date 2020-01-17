package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/armaanhammer/ble"
	"github.com/armaanhammer/ble/examples/lib/dev"
	"github.com/pkg/errors"
)

var (
	device = flag.String("device", "default", "implementation of ble")
	du     = flag.Duration("du", 5*time.Second, "advertising duration, 0 for indefinitely")
)

func main() {
	flag.Parse()

	d, err := dev.NewDevice("default")
	if err != nil {
		log.Fatalf("can't new device : %s", err)
	}
	ble.SetDefaultDevice(d)

	// Advertise for specified durantion, or until interrupted by user.
	fmt.Printf("Advertising for %s...\n", *du)
	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), *du))
	chkErr(ble.AdvertiseNameAndServices(ctx, "Gopher"))
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
