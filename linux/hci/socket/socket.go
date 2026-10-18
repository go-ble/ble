//go:build linux
// +build linux

package socket

import (
	"fmt"
	"io"
	"sync"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

func ioR(t, nr, size uintptr) uintptr {
	return (directionRead << directionShift) | (t << typeShift) | nr | (size << sizeShift)
}

func ioW(t, nr, size uintptr) uintptr {
	return (directionWrite << directionShift) | (t << typeShift) | nr | (size << sizeShift)
}

func ioctl(fd, op, arg uintptr) error {
	if _, _, ep := unix.Syscall(unix.SYS_IOCTL, fd, op, arg); ep != 0 {
		return ep
	}
	return nil
}

const (
	ioctlSize     = 4
	hciMaxDevices = 16
	typHCI        = 72 // 'H'
)

var (
	hciUpDevice      = ioW(typHCI, 201, ioctlSize) // HCIDEVUP
	hciDownDevice    = ioW(typHCI, 202, ioctlSize) // HCIDEVDOWN
	hciResetDevice   = ioW(typHCI, 203, ioctlSize) // HCIDEVRESET
	hciGetDeviceList = ioR(typHCI, 210, ioctlSize) // HCIGETDEVLIST
	hciGetDeviceInfo = ioR(typHCI, 211, ioctlSize) // HCIGETDEVINFO
)

type devListRequest struct {
	devNum     uint16
	devRequest [hciMaxDevices]struct {
		id  uint16
		opt uint32
	}
}

// Socket implements a HCI User Channel as ReadWriteCloser.
type Socket struct {
	fd     int
	id     int
	closed chan struct{}
	rmu    sync.Mutex
	wmu    sync.Mutex
}

// NewSocket returns a HCI User Channel of specified device id.
// If id is -1, the first available HCI device is returned.
func NewSocket(id int) (*Socket, error) {
	var err error
	// Create RAW HCI Socket.
	fd, err := unix.Socket(unix.AF_BLUETOOTH, unix.SOCK_RAW, unix.BTPROTO_HCI)
	if err != nil {
		return nil, errors.Wrap(err, "can't create socket")
	}

	if id != -1 {
		return open(fd, id)
	}

	req := devListRequest{devNum: hciMaxDevices}
	if err = ioctl(uintptr(fd), hciGetDeviceList, uintptr(unsafe.Pointer(&req))); err != nil {
		unix.Close(fd)
		return nil, errors.Wrap(err, "can't get device list")
	}
	var msg string
	for id := 0; id < int(req.devNum); id++ {
		s, err := open(fd, int(req.devRequest[id].id))
		if err == nil {
			return s, nil
		}
		msg = msg + fmt.Sprintf("(hci%d: %s)", id, err)
	}
	unix.Close(fd)
	return nil, errors.Errorf("no devices available: %s", msg)
}

func open(fd, id int) (*Socket, error) {
	// Reset the device in case previous session didn't cleanup properly.
	if err := ioctl(uintptr(fd), hciDownDevice, uintptr(id)); err != nil {
		return nil, errors.Wrap(err, "can't down device")
	}
	if err := ioctl(uintptr(fd), hciUpDevice, uintptr(id)); err != nil {
		return nil, errors.Wrap(err, "can't up device")
	}

	// HCI User Channel requires exclusive access to the device.
	// The device has to be down at the time of binding.
	if err := ioctl(uintptr(fd), hciDownDevice, uintptr(id)); err != nil {
		return nil, errors.Wrap(err, "can't down device")
	}

	// Bind the RAW socket to HCI User Channel
	sa := unix.SockaddrHCI{Dev: uint16(id), Channel: unix.HCI_CHANNEL_USER}
	if err := unix.Bind(fd, &sa); err != nil {
		return nil, errors.Wrap(err, "can't bind socket to hci user channel")
	}

	// poll for 20ms to see if any data becomes available, then clear it
	pfds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	unix.Poll(pfds, 20)
	if pfds[0].Revents&unix.POLLIN > 0 {
		b := make([]byte, 100)
		unix.Read(fd, b)
	}

	return &Socket{fd: fd, id: id, closed: make(chan struct{})}, nil
}

func (s *Socket) Read(p []byte) (int, error) {
	s.rmu.Lock()
	n, err := unix.Read(s.fd, p)
	s.rmu.Unlock()
	// Close always sends a dummy command to wake up Read
	// bad things happen to the HCI state machines if they receive
	// a reply from that command, so make sure no data is returned
	// on a closed socket.
	//
	// note that if Write and Close are called concurrently it's
	// indeterminate which replies get through.
	select {
	case <-s.closed:
		return 0, io.EOF
	default:
	}
	return n, errors.Wrap(err, "can't read hci socket")
}

func (s *Socket) Write(p []byte) (int, error) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	n, err := unix.Write(s.fd, p)
	return n, errors.Wrap(err, "can't write hci socket")
}

func (s *Socket) Close() error {
	close(s.closed)
	s.Write([]byte{0x01, 0x09, 0x10, 0x00}) // no-op command to wake up the Read call if it's blocked
	s.rmu.Lock()
	defer s.rmu.Unlock()
	err := unix.Close(s.fd)

	// Bring the adapter back up so BlueZ can re-acquire it.
	// Closing the HCI User Channel leaves the device down and invisible to
	// BlueZ D-Bus; an HCIDEVUP ioctl on a fresh raw socket re-registers it.
	if upFd, e := unix.Socket(unix.AF_BLUETOOTH, unix.SOCK_RAW, unix.BTPROTO_HCI); e == nil {
		ioctl(uintptr(upFd), hciUpDevice, uintptr(s.id))
		unix.Close(upFd)
	}

	return errors.Wrap(err, "can't close hci socket")
}
