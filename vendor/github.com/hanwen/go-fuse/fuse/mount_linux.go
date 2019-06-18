// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path"
	"syscall"
	"unsafe"

	cmount "github.com/containerd/containerd/mount"
)

func unixgramSocketpair() (l, r *os.File, err error) {
	fd, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, nil, os.NewSyscallError("socketpair",
			err.(syscall.Errno))
	}
	l = os.NewFile(uintptr(fd[0]), "socketpair-half1")
	r = os.NewFile(uintptr(fd[1]), "socketpair-half2")
	return
}

// Create a FUSE FS on the specified mount point.  The returned
// mount point is always absolute.
func mount(mountPoint string, opts *MountOptions, ready chan<- error) (fd int, err error) {
	user, err := user.Current()
	if err != nil {
		return 0, err
	}

	f, err := os.OpenFile("/dev/fuse", os.O_RDWR, 0666)
	if err != nil {
		return 0, err
	}
	fd = int(f.Fd())

	m := cmount.Mount{
		Type:   fmt.Sprintf("fuse.%s", opts.Name),
		Source: opts.FsName,
		Options: []string{
			"nosuid",
			"nodev",
			fmt.Sprintf("fd=%d", fd),
			fmt.Sprintf("rootmode=%#o", syscall.S_IFDIR),
			fmt.Sprintf("user_id=%s", user.Uid),
			fmt.Sprintf("group_id=%s", user.Gid),
		},
	}

	if opts.AllowOther {
		m.Options = append(m.Options, "allow_other")
	}

	m.Options = append(m.Options, opts.Options...)

	err = m.Mount(mountPoint)
	if err != nil {
		return 0, err
	}

	close(ready)
	return fd, err
}

func unmount(mountPoint string) (err error) {
	bin, err := fusermountBinary()
	if err != nil {
		return err
	}
	errBuf := bytes.Buffer{}
	cmd := exec.Command(bin, "-u", mountPoint)
	cmd.Stderr = &errBuf
	err = cmd.Run()
	if errBuf.Len() > 0 {
		return fmt.Errorf("%s (code %v)\n",
			errBuf.String(), err)
	}
	return err
}

func getConnection(local *os.File) (int, error) {
	var data [4]byte
	control := make([]byte, 4*256)

	// n, oobn, recvflags, from, errno  - todo: error checking.
	_, oobn, _, _,
		err := syscall.Recvmsg(
		int(local.Fd()), data[:], control[:], 0)
	if err != nil {
		return 0, err
	}

	message := *(*syscall.Cmsghdr)(unsafe.Pointer(&control[0]))
	fd := *(*int32)(unsafe.Pointer(uintptr(unsafe.Pointer(&control[0])) + syscall.SizeofCmsghdr))

	if message.Type != 1 {
		return 0, fmt.Errorf("getConnection: recvmsg returned wrong control type: %d", message.Type)
	}
	if oobn <= syscall.SizeofCmsghdr {
		return 0, fmt.Errorf("getConnection: too short control message. Length: %d", oobn)
	}
	if fd < 0 {
		return 0, fmt.Errorf("getConnection: fd < 0: %d", fd)
	}
	return int(fd), nil
}

// lookPathFallback - search binary in PATH and, if that fails,
// in fallbackDir. This is useful if PATH is possible empty.
func lookPathFallback(file string, fallbackDir string) (string, error) {
	binPath, err := exec.LookPath(file)
	if err == nil {
		return binPath, nil
	}

	abs := path.Join(fallbackDir, file)
	return exec.LookPath(abs)
}

func fusermountBinary() (string, error) {
	return lookPathFallback("fusermount", "/bin")
}

func umountBinary() (string, error) {
	return lookPathFallback("umount", "/bin")
}
