package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/fabian-z/habroker/scm"
)

type ConnectionError struct {
	conn net.Conn     // the conn which could not be passed to receiver
	addr scm.SockAddr //socket addr with which the error occured (dedup)
	err  error        // the error raised when passing conn
}

var config *Configuration // expected to remain static

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	err := systemdEnableWatchdog()
	if err != nil {
		log.Fatal(err)
	}

	config, err = ReadConfig()
	if err != nil {
		log.Fatal(err)
	}

	connChan := make(chan net.Conn, 1)
	errChan := make(chan *ConnectionError, 100)
	reloadChan := make(chan struct{}, 1)

	c := make(chan os.Signal, 1)
	signal.Notify(c,
		syscall.SIGHUP)

	go func() {
		for sig := range c {
			// TODO handle signal -> graceful shutdown
			log.Println("Got signal", sig)
			reloadChan <- struct{}{}
		}
	}()

	// TODO configuration
	listener, err := net.Listen("tcp", ":"+strconv.FormatUint(uint64(config.ListenPort), 10))
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Fatal(err)
			}
			connChan <- conn
		}
	}()

	for {
		rec, err := NewReceiver()
		if err != nil {
			log.Fatal(err)
		}

		err = rec.Start()
		if err != nil {
			log.Fatal(err)
		}

		ctx, cancelFunc := context.WithCancel(context.Background())

		go func(rec *Receiver) {
			for {
				select {
				case <-ctx.Done():
					return
				case conn := <-connChan:
					log.Println("Accepted connection from", conn.RemoteAddr().String())
					go func() {
						err := rec.Handle(conn)
						if err != nil {
							log.Println("error ", err)
							errChan <- &ConnectionError{
								conn: conn,
								addr: rec.addr,
								err:  err,
							}
						}
					}()
				}
			}
		}(rec)

		systemdNotify(SYSTEMD_NOTIFY_READY)

	inner:
		for {
			select {
			case <-reloadChan:
				log.Println("Reloading on request")
				// Type=notify-reload should work, but did not in tests
				systemdNotify(SYSTEMD_NOTIFY_RELOAD)
				cancelFunc()
				rec.Stop()
				break inner

			case connErr := <-errChan:
				log.Println("Connection error:", connErr.addr, connErr.err)

				if connErr.addr != rec.addr {
					// error occured with old receiver instance
					connChan <- connErr.conn
					continue
				}

				cancelFunc()
				rec.Stop()
				go func() {
					// Sending in a goroutine covers cases where buffer is full
					// without breaking inner loop, no receiver would run..
					connChan <- connErr.conn
				}()
				break inner
			}
		}

	}

}
