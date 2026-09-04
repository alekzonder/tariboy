//go:build darwin

package shim

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func waitChildExit(pid int) (err error) {
	queue, err := unix.Kqueue()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unix.Close(queue)) }()

	changes := []unix.Kevent_t{{
		Ident:  uint64(pid),
		Filter: unix.EVFILT_PROC,
		Flags:  unix.EV_ADD | unix.EV_ONESHOT,
		Fflags: unix.NOTE_EXIT,
	}}
	for {
		_, err = unix.Kevent(queue, changes, nil, nil)
		if !errors.Is(err, unix.EINTR) {
			break
		}
	}
	if err != nil {
		return err
	}

	events := make([]unix.Kevent_t, 1)
	for {
		var count int
		count, err = unix.Kevent(queue, nil, events, nil)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("wait for child exit: got %d events", count)
		}
		break
	}
	event := events[0]
	if event.Flags&unix.EV_ERROR != 0 {
		return syscall.Errno(event.Data)
	}
	if event.Ident != uint64(pid) || event.Filter != unix.EVFILT_PROC || event.Fflags&unix.NOTE_EXIT == 0 {
		return fmt.Errorf("wait for child exit: unexpected event")
	}
	return nil
}
