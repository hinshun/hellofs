.PHONY: hellofs

hellofs:
	@rm -rf ./mnt > /dev/null
	@mkdir ./mnt
	@GO111MODULE=off go run . ./mnt
