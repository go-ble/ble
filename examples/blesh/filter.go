package main

import (
	"strings"

	"github.com/armaanhammer/ble"
	"github.com/urfave/cli"
)

func filter(c *cli.Context) ble.AdvFilter {
	if c.String("name") != "" {
		return func(a ble.Advertisement) bool {
			return strings.ToLower(a.LocalName()) == strings.ToLower(c.String("name"))
		}
	}
	if c.String("addr") != "" {
		return func(a ble.Advertisement) bool {
			return a.Addr().String() == strings.ToLower(c.String("addr"))
		}
	}
	if svc := strings.ToLower(c.String("svc")); svc != "" {
		return func(a ble.Advertisement) bool {
			for _, s := range a.Services() {
				if s.String() == svc {
					return true
				}
			}
			return false
		}
	}
	return nil
}
