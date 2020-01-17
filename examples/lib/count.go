package lib

import (
	"fmt"
	"log"
	"time"

	"github.com/armaanhammer/ble"
)

// NewCountChar ...
func NewCountChar() *ble.Characteristic {
	n := 0
	c := ble.NewCharacteristic(CountCharUUID)
	c.HandleRead(ble.ReadHandlerFunc(func(req ble.Request, rsp ble.ResponseWriter) {
		fmt.Fprintf(rsp, "count: Read %d", n)
		log.Printf("count: Read %d", n)
		n++
	}))

	c.HandleWrite(ble.WriteHandlerFunc(func(req ble.Request, rsp ble.ResponseWriter) {
		log.Printf("count: Wrote %s", string(req.Data()))
	}))

	c.HandleNotify(ble.NotifyHandlerFunc(func(req ble.Request, n ble.Notifier) {
		cnt := 0
		log.Printf("count: Notification subscribed")
		for {
			select {
			case <-n.Context().Done():
				log.Printf("count: Notification unsubscribed")
				return
			case <-time.After(time.Second):
				log.Printf("count: Notify: %d", cnt)
				if _, err := fmt.Fprintf(n, "Count: %d", cnt); err != nil {
					// Client disconnected prematurely before unsubscription.
					log.Printf("count: Failed to notify : %s", err)
					return
				}
				cnt++
			}
		}
	}))

	c.HandleIndicate(ble.NotifyHandlerFunc(func(req ble.Request, n ble.Notifier) {
		cnt := 0
		log.Printf("count: Indication subscribed")
		for {
			select {
			case <-n.Context().Done():
				log.Printf("count: Indication unsubscribed")
				return
			case <-time.After(time.Second):
				log.Printf("count: Indicate: %d", cnt)
				if _, err := fmt.Fprintf(n, "Count: %d", cnt); err != nil {
					// Client disconnected prematurely before unsubscription.
					log.Printf("count: Failed to indicate : %s", err)
					return
				}
				cnt++
			}
		}
	}))
	return c
}
