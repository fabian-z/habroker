package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/fabian-z/habroker/scm"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	signal.Ignore(syscall.SIGINT, syscall.SIGHUP)

	scmConn, err := scm.DialSCM("@" + os.Args[1])

	if err != nil {
		log.Fatal(err)
	}
	defer scmConn.Close()

	var wg sync.WaitGroup

	for {
		conn, err := scmConn.ReadFD()
		if err != nil {
			// TODO can happen when SCM socket is closed (restart of frontend) -> shutdown
			break
		}

		wg.Add(1)

		go func() {
			tcpConn := conn.(*net.TCPConn)

			for i := 0; i <= 1000; i++ {
				_, err := tcpConn.Write([]byte("Hello from Go!\n"))
				if err != nil {
					break
				}
				time.Sleep(2 * time.Second)
			}
			tcpConn.Close()
			wg.Done()
		}()

	}

	wg.Wait()
}
