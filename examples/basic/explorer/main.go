package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/examples/lib/dev"
	"github.com/pkg/errors"
)

var (
	device = flag.String("device", "default", "implementation of ble")
	name   = flag.String("name", "Gopher", "name of remote peripheral")
	addr   = flag.String("addr", "", "address of remote peripheral (MAC on Linux, UUID on OS X)")
	sub    = flag.Duration("sub", 0, "subscribe to notification and indication for a specified period")
	sd     = flag.Duration("sd", 5*time.Second, "scanning duration, 0 for indefinitely")
)

func main() {
	flag.Parse()

	d, err := dev.NewDevice(*device)
	if err != nil {
		log.Fatalf("can't new device : %s", err)
	}
	ble.SetDefaultDevice(d)

	// Default to search device with name of Gopher (or specified by user).
	filter := func(a ble.Advertisement) bool {
		return strings.ToUpper(a.LocalName()) == strings.ToUpper(*name)
	}

	// If addr is specified, search for addr instead.
	if len(*addr) != 0 {
		filter = func(a ble.Advertisement) bool {
			return strings.ToUpper(a.Addr().String()) == strings.ToUpper(*addr)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	// Scan for specified durantion, or until interrupted by user.
	fmt.Printf("Scanning for %s...\n", *sd)
	connectCtx, cancelConnect := context.WithTimeout(ctx, *sd)
	cln, err := ble.Connect(connectCtx, filter)
	if err != nil {
		log.Fatalf("can't connect : %s", err)
	}
	cancelConnect()

	// Make sure we had the chance to print out the message.
	done := make(chan struct{})
	// Normally, the connection is disconnected by us after our exploration.
	// However, it can be asynchronously disconnected by the remote peripheral.
	// So we wait(detect) the disconnection in the go routine.
	go func() {
		<-cln.Disconnected()
		fmt.Printf("[ %s ] is disconnected \n", cln.Addr())
		close(done)
	}()

	fmt.Printf("Discovering profile...\n")
	p, err := cln.DiscoverProfile(true)
	if err != nil {
		log.Fatalf("can't discover profile: %s", err)
	}

	// Start the exploration.
	explore(ctx, cln, p)

	// Disconnect the connection. (On OS X, this might take a while.)
	fmt.Printf("Disconnecting [ %s ]... (this might take up to few seconds on OS X)\n", cln.Addr())
	cln.CancelConnection()

	<-done
}

func explore(ctx context.Context, cln ble.Client, p *ble.Profile) error {
	var activeSubscriptions []*ble.Characteristic // Wait at the end if the list is not empty.

	for _, s := range p.Services {
		fmt.Printf("    Service: %s %s, Handle (0x%02X)\n", s.UUID, ble.Name(s.UUID), s.Handle)

		for _, c := range s.Characteristics {
			fmt.Printf("      Characteristic: %s %s, Property: 0x%02X (%s), Handle(0x%02X), VHandle(0x%02X)\n",
				c.UUID, ble.Name(c.UUID), c.Property, propString(c.Property), c.Handle, c.ValueHandle)
			if (c.Property & ble.CharRead) != 0 {
				b, err := cln.ReadCharacteristic(c)
				if err != nil {
					fmt.Printf("Failed to read characteristic: %s\n", err)
					continue
				}
				fmt.Printf("        Value         %x | %q\n", b, b)
			}

			for _, d := range c.Descriptors {
				fmt.Printf("        Descriptor: %s %s, Handle(0x%02x)\n", d.UUID, ble.Name(d.UUID), d.Handle)
				b, err := cln.ReadDescriptor(d)
				if err != nil {
					fmt.Printf("Failed to read descriptor: %s\n", err)
					continue
				}
				fmt.Printf("        Value         %x | %q\n", b, b)
			}

			if *sub != 0 {
				// Don't bother to subscribe the Service Changed characteristics.
				if c.UUID.Equal(ble.ServiceChangedUUID) {
					continue
				}

				// Don't touch the Apple-specific Service/Characteristic.
				// Service: D0611E78BBB44591A5F8487910AE4366
				// Characteristic: 8667556C9A374C9184ED54EE27D90049, Property: 0x18 (WN),
				//   Descriptor: 2902, Client Characteristic Configuration
				//   Value         0000 | "\x00\x00"
				if c.UUID.Equal(ble.MustParse("8667556C9A374C9184ED54EE27D90049")) {
					continue
				}

				prefix := fmt.Sprintf("Service(%s), Characteristic(%s): ", s.UUID, c.UUID)
				if (c.Property & ble.CharNotify) != 0 {
					fmt.Printf("\n-- Subscribe to notification for %s --\n", *sub)
					h := func(req []byte) { fmt.Printf("%s: Notified: %q [ % X ]\n", prefix, string(req), req) }
					if err := cln.Subscribe(c, false, h); err != nil {
						log.Fatalf("subscribe failed: %s", err)
					}
					activeSubscriptions = append(activeSubscriptions, c)
				}
				if (c.Property & ble.CharIndicate) != 0 {
					fmt.Printf("\n-- Subscribe to indication of %s --\n", *sub)
					h := func(req []byte) { fmt.Printf("%s: Indicated: %q [ % X ]\n", prefix, string(req), req) }
					if err := cln.Subscribe(c, true, h); err != nil {
						log.Fatalf("subscribe failed: %s", err)
					}
					activeSubscriptions = append(activeSubscriptions, c)
				}
			}
		}
		fmt.Printf("\n")
	}
	if len(activeSubscriptions) == 0 {
		return nil
	}

	// If we are subscribed, wait out the subscription duration (--sub).
	select {
	case <-time.After(*sub): // The happy path.
	case <-cln.Disconnected():
		// Some devices will kick us earlier. Maybe reconnect.
		// In this example we will proceed, unsubscribing will not block.
	case <-ctx.Done():
		// The user has pressed ctrl+C or something.
		// This could be the user connecting to a different endpoint.
		// In any case, show how to orderly exit by proceeding to cln.Unsubscribe.
	}
	var err error // Retain the last error, do not exit on the first with entries remaining.
	for _, c := range activeSubscriptions {
		if err2 := cln.Unsubscribe(c, (c.Property&ble.CharNotify) != 0); err2 != nil {
			err = err2
			log.Printf("unsubscribe failed: %s", err2)
		} else {
			log.Printf("-- Unsubscribed from notification or indication --\n")
		}
	}
	return err
}

func propString(p ble.Property) string {
	var s string
	for k, v := range map[ble.Property]string{
		ble.CharBroadcast:   "B",
		ble.CharRead:        "R",
		ble.CharWriteNR:     "w",
		ble.CharWrite:       "W",
		ble.CharNotify:      "N",
		ble.CharIndicate:    "I",
		ble.CharSignedWrite: "S",
		ble.CharExtended:    "E",
	} {
		if p&k != 0 {
			s += v
		}
	}
	return s
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
