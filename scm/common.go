package scm

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"os"
	"syscall"
)

type SCMConn struct {
	*net.UnixConn
}

type SCMListener struct {
	*net.UnixListener
}

const (
	ID_LEN = 32
)

type SockAddr string

func GenerateAddress() SockAddr {
	var randAddr [ID_LEN]byte

	_, err := rand.Read(randAddr[:])
	if err != nil {
		panic(err)
	}

	return SockAddr("@" + base64.RawURLEncoding.EncodeToString(randAddr[:]))
}

func (s SockAddr) String() string {
	return string(s)
}

func (s SockAddr) ID() string {
	return string(s[1:])
}

func ListenSCM(path string) (*SCMListener, error) {

	isAbstract := path[0] == byte('@')

	if !isAbstract {
		syscall.Unlink(path)
	}

	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, err
	}

	ul, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, err
	}

	if !isAbstract {
		err = os.Chmod(path, 0777)
		if err != nil {
			return nil, err
		}
	}

	return &SCMListener{ul}, nil
}

func (l *SCMListener) Accept() (*SCMConn, error) {
	uc, err := l.AcceptUnix()
	if err != nil {
		return nil, err
	}

	return &SCMConn{uc}, nil
}

func DialSCM(path string) (*SCMConn, error) {

	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		return nil, err
	}

	return &SCMConn{conn}, nil
}

func (c *SCMConn) WriteFD(conn net.Conn) error {

	var raw syscall.RawConn
	var err error

	switch conn := conn.(type) {
	case *net.TCPConn:
		raw, err = conn.SyscallConn()
	case *net.UDPConn:
		raw, err = conn.SyscallConn()
	default:
		return errors.New("unsupported connection type")
	}

	if err != nil {
		return err
	}

	var dupFD int

	raw.Control(func(fd uintptr) {
		dupFD, err = syscall.Dup(int(fd))
	})

	if err != nil {
		return err
	}

	fds := syscall.UnixRights(dupFD)

	_, oobn, err := c.WriteMsgUnix(nil, fds, nil)

	if err != nil {
		return err
	}

	if oobn < len(fds) {
		return io.ErrShortWrite
	}

	err = conn.Close()
	if err != nil {
		return err
	}

	// At this point, only the dup'd FD remains with single reference

	return nil
}

func (c *SCMConn) ReadFD() (net.Conn, error) {
	msg, oob := make([]byte, 2), make([]byte, 128)

	_, oobn, _, _, err := c.ReadMsgUnix(msg, oob)
	if err != nil {
		return nil, err
	}

	cmsgs, err := syscall.ParseSocketControlMessage(oob[0:oobn])
	if err != nil {
		return nil, err
	} else if len(cmsgs) != 1 {
		return nil, errors.New("invalid number of cmsgs received")
	}

	fds, err := syscall.ParseUnixRights(&cmsgs[0])
	if err != nil {
		return nil, err
	} else if len(fds) != 1 {
		return nil, errors.New("invalid number of fds received")
	}

	fd := os.NewFile(uintptr(fds[0]), "")
	if fd == nil {
		return nil, errors.New("could not open fd")
	}

	conn, err := net.FileConn(fd)
	return conn, err
}
