.PHONY: hellofs

hellofs:
	@rm -rf ./mnt > /dev/null
	@mkdir ./mnt
	@GO111MODULE=off go build
	@sudo ./hellofs ./mnt

umount:
	@fusermount -u ./mnt | true
	@sudo umount ./mnt > /dev/null | true
