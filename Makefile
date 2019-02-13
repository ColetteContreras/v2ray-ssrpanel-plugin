.PHONEY: build
build:
	go build -o /tmp/v2ray_ssrpanel_plugin.so -buildmode=plugin ./main

.PHONEY: release
release:
	cd ${GOPATH}/src/v2ray.com/core \
	&& bazel clean \
	&& bazel build --action_env=GOPATH=${GOPATH} --action_env=PATH=${PATH} \
		//release:v2ray_darwin_amd64_package \
		//release:v2ray_linux_amd64_package \
		//release:v2ray_linux_arm_package
