hellofs
---

Demonstrating mounting a FUSE filesystem using `github.com/containerd/containerd/mount` instead of `fusermount`.

# hanwen/go-fuse changes

https://github.com/hinshun/hellofs/blob/master/vendor/github.com/hanwen/go-fuse/fuse/mount_linux.go#L31-L71
```golang
// ...
import (
	// ...
	cmount "github.com/containerd/containerd/mount"
)
// ...

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
```

# FUSE Overview

FUSE is an userspace filesystem framework, it consists of a kernel module `fuse.ko` that has a FUSE VFS interfaced by a device `/dev/fuse` on your system. Many FUSE implementations rely on `libfuse` (a C library to interact with `/dev/fuse` and its protocol), and `fusermount` (a binary owned by `root` but executable by users with a `suid` bit set, so that users can use the mount FUSE filesystems without being root). Libraries like [github.com/hanwen/go-fuse](https://github.com/hanwen/go-fuse) and [github.com/bazil/fuse](https://github.com/bazil/fuse) use `fusermount` to mount but not `libfuse`.

`fusermount` is a binary commonly used to mount FUSE filesystems. FUSE implementers typically call `fusermount` as a subprocess, and provides a binary like `sshfs` that users invoke to mount the filesystem instead of calling `fusermount`, `mount(8)` or `mount(2)` directly. The subprocess needs to have an environment variable `_FUSE_COMMFD` set. The FUSE implementation needs to create a file descriptor, set `_FUSE_COMMFD=<fd>`, which `fusermount` will use to pass back the control fd open on `/dev/fuse`.

[`mount(8)`](http://man7.org/linux/man-pages/man8/mount.8.html) is a binary that mounts a filesystem. Internally it uses `mount(2)` the syscall in order to actually get the kernel to mount. `mount(8)` registers filesystems using executables with the name `/sbin/mount.*`. These executables need to fulfill a specific interface, that `mount(8)` will invoke on the `/sbin/mount.*` binary. You can add a `/sbin/mount.<your-fuse-mount-wrapper>` that should daemonize a process running your FUSE server. However, `mount(2)` will not know about these `/sbin/mount.*` binaries, this is just an implementation detail of `mount(8)`. There is a `/sbin/mount.fuse` that one of these mount wrappers that will execute `fusermount` under the hood.

[`mount(2)`](http://man7.org/linux/man-pages/man2/mount.2.html) is the mount syscall. It only knows about filesystems registered by the kernel ([register_filesystem](https://www.kernel.org/doc/htmldocs/filesystems/API-register-filesystem.html)), which are visible via `cat /proc/filesystems`. However, there seems to be undocumented behaviour in that if you call `mount(2)` with a type is prefixed like `fuse.<subtype>`, it will actually mount via FUSE. The `subtype` doesn't seem to actually matter other than being the type of the mount when you run `mount -l`.

The `mount(2)` signature looks like this:
```c
       #include <sys/mount.h>

       int mount(const char *source, const char *target,
                 const char *filesystemtype, unsigned long mountflags,
                 const void *data);
```

For `filesystemtype` of `fuse.*` type:
- `source` is unused, you can use it to supply the FUSE name like `sshfs`.
- `target` is the mountpoint
- `filesystemtype` looks like `fuse.*` (i.e. `fuse.sshfs`).
- `mountflags` are flags you can find on `mount(2)`'s manpage. If mountflags is empty, then it creates a mount. You can supply `mountflags` to run `mount(2)` on existing mounts to change properties like making it readonly, change propagation types, etc. In some of those cases, `source`, `filesystemtype` or `filesystemtype` and `data` are ignored.
- `data` is an optional buffer to provide filesystem specific options.

For `fuse.*` filesystem types, the options for `data` are delimited by comma, and must have these 4(?) fields:
- `fd` is the file descriptor you get when you open `/dev/fuse` with `O_RDWR`, this is the control FD where FUSE protocol messages are sent between the FUSE VFS in the kernel, and your FUSE server in userspace.
- `rootmode` is file mode of your mountpoint bitwise AND with `S_IFMT`, its a bitmask to show only bits of the file mode that says if its a regular, directory, device file, etc. I believe it expects it to be `S_IFDIR`, which is the bit that represents it being a directory.
- `user_id` is the uid of the user mounting.
- `group_id` is the gid of the user mounting.

So the FUSE workflow is:
1. Open `/dev/fuse` with `O_RDWR` to produce `control FD`.
2. `mount(2)` with `source=<fuse-name>`, `target=<mountpoint>`, `filesystemtype=fuse.<fuse-name>`, `mountflags=0`, `data=fd=<control FD>,rootmode=<S_IFDIR in octal str>,user_id=<uid>,group_id=<gid>`.
3. FUSE server reads initialization request from `control FD` and responds with some data about its implementation.
4. FUSE starts listening on `control FD`.
5. I/O performed on file in `mountpoint` is handled by FUSE VFS in kernel, which sends a request to FUSE server via `control FD`.
6. FUSE server responds via `control FD`, to complete the I/O.
