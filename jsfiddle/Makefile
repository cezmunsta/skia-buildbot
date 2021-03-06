build:
	npx webpack --mode=development

serve:
	npx webpack-dev-server --watch-poll --mode=development

release_build: wasm_libs
	npx webpack --mode=production

watch:
	npx webpack --watch --mode=development

get_latest_skia:
	docker pull gcr.io/skia-public/skia-wasm-release:prod

wasm_libs:
	rm -rf build
	mkdir -p build/canvaskit build/pathkit
	# Extract them from the most recently built ones. These are built
	# using cloudbuild.yaml in the Skia repo.
	docker run --rm --volume `pwd`/build:/OUT gcr.io/skia-public/skia-wasm-release:prod sh -c "cp -r /tmp/* /OUT/"
	echo "export const SKIA_VERSION = '`cat build/VERSION`';" > build/version.js

core:
	go install -v ./go/jsfiddle

release: release_build
	CGO_ENABLED=0 GOOS=linux go install -a ./go/jsfiddle
	./build_release

release_ci:
	cd ../infra-sk && npm ci
	npm ci
	# Assumes the cloud builder environment mounts the wasm outputs to /OUT
	rm -rf build
	mkdir -p build/canvaskit build/pathkit
	cp -r /OUT/* build
	echo "export const SKIA_VERSION = '`cat /OUT/VERSION`';" > ./build/version.js
	# Now that we have the canvaskit.wasm etc, we can bundle it in with webpack.
	npx webpack --mode=production
	CGO_ENABLED=0 GOOS=linux go install -a ./go/jsfiddle
	./build_release

push: get_latest_skia release
	pushk jsfiddle

.PHONY: build serve release_build watch core release wasm_libs get_latest_skia
