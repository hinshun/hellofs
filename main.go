package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hanwen/go-fuse/fs"
	"github.com/hanwen/go-fuse/fuse"
)

type HelloRoot struct {
	fs.Inode
}

func (r *HelloRoot) OnAdd(ctx context.Context) {
	ch := r.NewPersistentInode(
		ctx, &fs.MemRegularFile{
			Data: []byte("hello"),
			Attr: fuse.Attr{
				Mode: 0644,
			},
		}, fs.StableAttr{Ino: 2})
	r.AddChild("hello", ch, false)
}

func (r *HelloRoot) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0755
	return 0
}

var _ = (fs.NodeGetattrer)((*HelloRoot)(nil))
var _ = (fs.NodeOnAdder)((*HelloRoot)(nil))

func main() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  hello MOUNTPOINT")
	}

	err := run(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "hellofs: %s", err)
		os.Exit(1)
	}
}

func run(mountpoint string) error {
	// Catch SIGINT to unmount.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	log.Printf("Mounting hellofs on %q", mountpoint)
	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			AllowOther: true,
		},
	}
	server, err := fs.Mount(mountpoint, &HelloRoot{}, opts)
	if err != nil {
		return err
	}

	go func() {
		<-c
		log.Printf("Unmounting %q", mountpoint)
		err := server.Unmount()
		if err != nil {
			panic(err)
		}
	}()

	server.Wait()
	return nil
}
