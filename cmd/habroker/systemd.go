package main

import (
	"errors"
	"log"
	"os"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"github.com/coreos/go-systemd/v22/daemon"
)

func runningSystemd() bool {
	b, err := strconv.ParseBool(os.Getenv("SYSTEMD"))
	if err != nil || (!b) {
		return false
	} else {
		return true
	}
}

type SystemdNotification int

const (
	SYSTEMD_NOTIFY_INVALID SystemdNotification = iota
	SYSTEMD_NOTIFY_READY
	SYSTEMD_NOTIFY_RELOAD
	SYSTEMD_NOTIFY_WATCHDOG
	SYSTEMD_NOTIFY_BARRIER
)

func (s SystemdNotification) String() string {
	switch s {
	case SYSTEMD_NOTIFY_READY:
		return daemon.SdNotifyReady
	case SYSTEMD_NOTIFY_RELOAD:
		// Newline is needed as separator (taken from example for FDSTORE)
		return daemon.SdNotifyReloading + "\nMONOTONIC_USEC=" + strconv.FormatUint(monotimeUsec(), 10)
	case SYSTEMD_NOTIFY_WATCHDOG:
		return daemon.SdNotifyWatchdog
	case SYSTEMD_NOTIFY_BARRIER:
		return "BARRIER=1"
	default:
		panic("invalid notification")
	}
}

func systemdNotify(notification SystemdNotification) error {
	if !runningSystemd() {
		return nil
	}
	str := notification.String()
	sent, err := daemon.SdNotify(false, str)
	if err != nil {
		return err
	}
	if !sent {
		return errors.New("notification not supported / sent")
	}
	return nil
}

func systemdEnableWatchdog() error {
	if !runningSystemd() {
		return nil
	}

	usec, err := strconv.ParseInt(os.Getenv("WATCHDOG_USEC"), 10, 64)
	if err != nil {
		return err
	}

	ticker := time.NewTicker((time.Duration(usec) / 2) * time.Microsecond)
	go func() {
		for range ticker.C {
			systemdNotify(SYSTEMD_NOTIFY_WATCHDOG)
		}
	}()

	return nil
}

const (
	// Linux specific, but so is all this code
	CLOCK_REALTIME  = 0
	CLOCK_MONOTONIC = 1
)

func monotimeUsec() uint64 {
	var ts syscall.Timespec

	_, _, errno := syscall.Syscall(syscall.SYS_CLOCK_GETTIME, CLOCK_MONOTONIC, uintptr(unsafe.Pointer(&ts)), 0)
	if errno != 0 {
		log.Fatal(errno)
	}

	var usec uint64
	usec += uint64(ts.Sec) * 1e6
	usec += uint64(ts.Nsec) / 1e3

	return usec
}
