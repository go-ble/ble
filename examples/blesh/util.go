package main

import (
	"context"
	"fmt"

	"github.com/armaanhammer/ble"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func doGetUUID(c *cli.Context) error {
	if c.String("uuid") != "" {
		u, err := ble.Parse(c.String("uuid"))
		if err != nil {
			return errInvalidUUID
		}
		curr.uuid = u
	}
	if curr.uuid == nil {
		return errNoUUID
	}
	return nil
}

func doConnect(c *cli.Context) error {
	if c.String("addr") != "" {
		curr.addr = ble.NewAddr(c.String("addr"))
		curr.client = curr.clients[curr.addr.String()]
	}
	if curr.client != nil {
		return nil
	}
	return cmdConnect(c)
}

func doDiscover(c *cli.Context) error {
	if curr.profile != nil {
		return nil
	}
	return cmdDiscover(c)
}

func chkErr(err error) error {
	switch errors.Cause(err) {
	case context.DeadlineExceeded:
		// Sepcified duration passed, which is the expected case.
		return nil
	case context.Canceled:
		fmt.Printf("\n(Canceled)\n")
		return nil
	}
	return err
}
