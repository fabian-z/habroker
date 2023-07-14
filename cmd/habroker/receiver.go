package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"syscall"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/fabian-z/habroker/scm"
)

type Receiver struct {
	listener   *scm.SCMListener
	connection *scm.SCMConn
	process    *os.Process
	addr       scm.SockAddr
	path       string
	unit       string
}

func NewReceiver() (*Receiver, error) {

	var r Receiver
	var err error

	r.path = config.ReceiverPath
	r.unit = config.ReceiverUnit

	r.addr = scm.GenerateAddress()
	r.listener, err = scm.ListenSCM(r.addr.String())

	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (r *Receiver) GetPeer() (pid, uid, gid uint64, err error) {

	var ucred *syscall.Ucred
	var sysConn syscall.RawConn

	sysConn, err = r.connection.UnixConn.SyscallConn()
	if err != nil {
		return
	}

	// TODO verify uid / gid against configured values!
	// nedeed to secure access to the socket
	sysConn.Control(func(fd uintptr) {
		ucred, err = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})

	if err != nil {
		return
	}

	pid = uint64(ucred.Pid)
	uid = uint64(ucred.Uid)
	gid = uint64(ucred.Gid)

	return
}

func (r *Receiver) Start() error {
	var err error
	addrString := r.addr.ID()
	var childPid uint64

	if runningSystemd() {
		ctx := context.Background()
		systemdConn, err := dbus.NewSystemConnectionContext(ctx)
		if err != nil {
			log.Fatal(err)
		}
		defer systemdConn.Close()

		resChan := make(chan string, 1)
		target := r.unit + "@" + addrString + ".service"
		_, err = systemdConn.StartUnitContext(ctx, target, "replace", resChan)
		if err != nil {
			return err
		}
		res := <-resChan
		if res != "done" {
			return errors.New("systemd result is: " + res)
		}

		prop, err := systemdConn.GetServicePropertyContext(ctx, target, "MainPID")
		if err != nil {
			return err
		}

		switch val := prop.Value.Value().(type) {
		case uint:
			childPid = uint64(val)
		case uint32:
			childPid = uint64(val)
		case uint64:
			childPid = val
		default:
			log.Fatalf("Invalid MainPID type %T", prop.Value.Value())
		}

		log.Println("Started unit", target)

	} else {
		r.process, err = os.StartProcess(r.path, []string{"receiver", addrString}, &os.ProcAttr{
			Dir:   "",
			Env:   []string{},
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
			Sys: &syscall.SysProcAttr{
				Pdeathsig: syscall.SIGHUP,
			},
		})

		if err != nil {
			return err
		}

		go r.process.Wait()
		childPid = uint64(r.process.Pid)
		log.Println("Started process ", r.process.Pid)
	}

	// detach from process not necessary when setting Pdeathsig appropriately

	r.connection, err = r.listener.Accept()
	if err != nil {
		return err
	}

	pid, uid, gid, err := r.GetPeer()
	if err != nil {
		return err
	}

	if pid != childPid {
		r.connection.Close()
		r.listener.Close()
		return errors.New("unexpected peer")
	}

	log.Printf("Connection from expected pid with uid %v, gid %v", uid, gid)

	return nil
}

func (r *Receiver) Handle(conn net.Conn) error {
	err := r.connection.WriteFD(conn)
	// no close necessary
	if err != nil {
		return err
	}
	return nil
}

func (r *Receiver) Stop() error {
	err := r.listener.Close()
	if err != nil {
		return err
	}
	return r.connection.Close()
}
